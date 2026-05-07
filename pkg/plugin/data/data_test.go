package data_test

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/data/component"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Test fixtures
// -------------------------------------------------------------------------------------------------

//go:embed testdata/abilities.json testdata/resolver_side.json
var testFS embed.FS

// AbilityRecord matches a single record in testdata/abilities.json.
type AbilityRecord struct {
	ID       string  `json:"id"`
	Cooldown float64 `json:"cooldown"`
}

// Abilities is the primary test Definition.
type Abilities struct {
	Items []AbilityRecord `json:"items"`
}

func (Abilities) Name() string     { return "test_abilities" }
func (Abilities) JSONFile() string { return "testdata/abilities.json" }

// -------------------------------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------------------------------

func newWorld(t *testing.T) *cardinal.World {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")
	debug := true
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		Region:              "local",
		Organization:        "data-test",
		Project:             "data-test",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1_000_000,
		Debug:               &debug,
	})
	require.NoError(t, err)
	return w
}

// initCardinalECS runs Init-hook systems on Cardinal's embedded ecs.World. The shard loop
// (cardinal.World.run) does this between RegisterPlugin and the first Tick, but unit tests can't
// call run(). Cardinal's ecs world is unexported, so we reach in via reflection.
//
// Same pattern as physics2d/test/utils_test.go's initCardinalECS.
func initCardinalECS(t *testing.T, w *cardinal.World) {
	t.Helper()
	v := reflect.ValueOf(w).Elem()
	f := v.FieldByName("world")
	require.True(t, f.IsValid(), "cardinal.World: missing embedded ecs world field")
	inner := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	m := inner.MethodByName("Init")
	require.True(t, m.IsValid(), "ecs.World: missing Init method")
	m.Call(nil)
}

func tickOnce(t *testing.T, w *cardinal.World) {
	t.Helper()
	w.Tick(context.Background(), time.Unix(0, 0))
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// manifestSearch is the Exact search shape used by the test harness systems below. Declaring it
// in a system state struct registers component.ConfigManifest with Cardinal so the test's
// pre-seed/observer systems can read and write the same singleton as the plugin's reconcile.
type manifestSearch = cardinal.Exact[struct {
	Item cardinal.Ref[component.ConfigManifest]
}]

type manifestObserverState struct {
	cardinal.BaseSystemState
	Manifest manifestSearch
}

type manifestPreseedState struct {
	cardinal.BaseSystemState
	Manifest manifestSearch
}

// -------------------------------------------------------------------------------------------------
// Fake sources
// -------------------------------------------------------------------------------------------------

// versionedFake is a versioned source: it holds multiple content versions per file, addressed by
// hash. Mimics OperatorSource — pin-by-hash works, missing-hash errors.
type versionedFake struct {
	current map[string]string // file → hash of the version this source currently serves
	byHash  map[string][]byte // hash → bytes
}

func (f *versionedFake) Fetch(_ context.Context, file, hash string) ([]byte, string, error) {
	if hash == "" {
		h, ok := f.current[file]
		if !ok {
			return nil, "", errors.New("versionedFake: file not currently served: " + file)
		}
		hash = h
	}
	bytes, ok := f.byHash[hash]
	if !ok {
		return nil, "", errors.New("versionedFake: no blob for hash " + hash)
	}
	return bytes, hash, nil
}

// singleVersionFake mimics EmbedSource: only one version exists; the requested hash is ignored
// and current bytes are returned. The caller (the plugin's reconcile) detects this by comparing
// the returned hash to what it asked for.
type singleVersionFake struct {
	files map[string][]byte
}

func (f *singleVersionFake) Fetch(_ context.Context, file, _ string) ([]byte, string, error) {
	bytes, ok := f.files[file]
	if !ok {
		return nil, "", errors.New("singleVersionFake: file not present: " + file)
	}
	return bytes, sha256hex(bytes), nil
}

// errFake always errors. Used to test the Register-time fetch failure path.
type errFake struct{}

func (errFake) Fetch(_ context.Context, _, _ string) ([]byte, string, error) {
	return nil, "", errors.New("synthetic fetch failure")
}

// -------------------------------------------------------------------------------------------------
// Tests — basic load
// -------------------------------------------------------------------------------------------------

// TestPlugin_LoadsRegisteredKind verifies Get[T] returns the loaded value immediately after
// cardinal.RegisterPlugin — the catalog loads eagerly in Plugin.Register, no Init/Tick needed.
func TestPlugin_LoadsRegisteredKind(t *testing.T) {
	w := newWorld(t)

	plugin := data.NewPlugin(data.Config{EmbeddedFS: testFS})
	data.Register[Abilities](plugin)
	cardinal.RegisterPlugin(w, plugin)

	got := data.Get[Abilities]()
	require.Equal(t, []AbilityRecord{
		{ID: "fireball", Cooldown: 2.5},
		{ID: "frostbolt", Cooldown: 3.0},
	}, got.Items)
}

// -------------------------------------------------------------------------------------------------
// Tests — manifest component lifecycle
// -------------------------------------------------------------------------------------------------

// TestPlugin_ManifestComponentOnFreshWorld verifies the reconcile system creates a ConfigManifest
// singleton on the first tick of a fresh world, populated from the plugin's per-file manifest.
func TestPlugin_ManifestComponentOnFreshWorld(t *testing.T) {
	w := newWorld(t)

	plugin := data.NewPlugin(data.Config{EmbeddedFS: testFS})
	data.Register[Abilities](plugin)
	cardinal.RegisterPlugin(w, plugin)

	abilitiesBytes, err := testFS.ReadFile("testdata/abilities.json")
	require.NoError(t, err)
	expectedHash := sha256hex(abilitiesBytes)

	var observed component.ConfigManifest
	var observeErr error
	cardinal.RegisterSystem(w, func(state *manifestObserverState) {
		_, ent, err := state.Manifest.Iter().Single()
		observeErr = err
		if err == nil {
			observed = ent.Item.Get()
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(t, w)
	tickOnce(t, w)

	require.NoError(t, observeErr, "expected ConfigManifest singleton to exist after first tick")
	require.Equal(t, map[string]string{"testdata/abilities.json": expectedHash}, observed.Files)
}

// TestPlugin_ReconcileReFetchesChangedFileAtSnapshotHash simulates a snapshot restore whose
// manifest references an OLD hash. The plugin loaded the source's CURRENT version at Register;
// the reconcile pass on the first tick must detect the mismatch and re-fetch the old version
// from the source. Asserts the catalog ends up holding the old bytes and the ConfigManifest
// component is left equal to the post-reconcile manifest.
func TestPlugin_ReconcileReFetchesChangedFileAtSnapshotHash(t *testing.T) {
	w := newWorld(t)

	v1Bytes := []byte(`{"items":[{"id":"oldfire","cooldown":1.0}]}`)
	v2Bytes := []byte(`{"items":[{"id":"newfire","cooldown":2.0}]}`)
	h1 := sha256hex(v1Bytes)
	h2 := sha256hex(v2Bytes)

	src := &versionedFake{
		current: map[string]string{"testdata/abilities.json": h2},
		byHash:  map[string][]byte{h1: v1Bytes, h2: v2Bytes},
	}

	plugin := data.NewPlugin(data.Config{Source: src})
	data.Register[Abilities](plugin)
	cardinal.RegisterPlugin(w, plugin)

	// Sanity: plugin loaded the current (v2) at Register.
	require.Equal(t, []AbilityRecord{{ID: "newfire", Cooldown: 2.0}}, data.Get[Abilities]().Items)

	// Pre-seed at Init: a ConfigManifest referencing the OLD hash, as if restored from a snapshot.
	cardinal.RegisterSystem(w, func(state *manifestPreseedState) {
		_, ent := state.Manifest.Create()
		ent.Item.Set(component.ConfigManifest{Files: map[string]string{"testdata/abilities.json": h1}})
	}, cardinal.WithHook(cardinal.Init))

	var observed component.ConfigManifest
	cardinal.RegisterSystem(w, func(state *manifestObserverState) {
		_, ent, err := state.Manifest.Iter().Single()
		if err == nil {
			observed = ent.Item.Get()
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(t, w)
	tickOnce(t, w)

	// Catalog reconciled to v1 content.
	require.Equal(t, []AbilityRecord{{ID: "oldfire", Cooldown: 1.0}}, data.Get[Abilities]().Items)
	// Component reflects the reconciled manifest (now equal to plugin.manifest).
	require.Equal(t, map[string]string{"testdata/abilities.json": h1}, observed.Files)
}

// TestPlugin_EmbedMismatchWarnsAndKeepsCurrent simulates a rebuilt-binary case: a single-version
// source's bytes differ from what the snapshot's manifest claims. Reconcile must keep the current
// catalog and rewrite the ConfigManifest component to the plugin's current manifest so the next
// snapshot is self-consistent and the warning doesn't re-fire on subsequent ticks.
func TestPlugin_EmbedMismatchWarnsAndKeepsCurrent(t *testing.T) {
	w := newWorld(t)

	currentBytes := []byte(`{"items":[{"id":"current","cooldown":1.0}]}`)
	currentHash := sha256hex(currentBytes)
	src := &singleVersionFake{files: map[string][]byte{"testdata/abilities.json": currentBytes}}

	plugin := data.NewPlugin(data.Config{Source: src})
	data.Register[Abilities](plugin)
	cardinal.RegisterPlugin(w, plugin)

	// Pre-seed a manifest with a stale hash the source cannot serve (it'll return currentBytes
	// regardless of requested hash → gotHash != requested → warn path).
	const staleHash = "0000000000000000000000000000000000000000000000000000000000000000"
	cardinal.RegisterSystem(w, func(state *manifestPreseedState) {
		_, ent := state.Manifest.Create()
		ent.Item.Set(component.ConfigManifest{Files: map[string]string{"testdata/abilities.json": staleHash}})
	}, cardinal.WithHook(cardinal.Init))

	var observed component.ConfigManifest
	cardinal.RegisterSystem(w, func(state *manifestObserverState) {
		_, ent, err := state.Manifest.Iter().Single()
		if err == nil {
			observed = ent.Item.Get()
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(t, w)
	tickOnce(t, w)

	// Catalog unchanged (current bytes).
	require.Equal(t, []AbilityRecord{{ID: "current", Cooldown: 1.0}}, data.Get[Abilities]().Items)
	// Component rewritten to current hash.
	require.Equal(t, map[string]string{"testdata/abilities.json": currentHash}, observed.Files)

	// Second tick: steady state, component still matches plugin manifest.
	tickOnce(t, w)
	require.Equal(t, map[string]string{"testdata/abilities.json": currentHash}, observed.Files)
}

// TestPlugin_ReconcileFailurePanicsOnVersionedSource verifies the fail-loud path: when a versioned
// source returns an error for a hash the snapshot says was running, the reconcile system panics.
func TestPlugin_ReconcileFailurePanicsOnVersionedSource(t *testing.T) {
	w := newWorld(t)

	currentBytes := []byte(`{"items":[{"id":"current","cooldown":1.0}]}`)
	hCurrent := sha256hex(currentBytes)
	src := &versionedFake{
		current: map[string]string{"testdata/abilities.json": hCurrent},
		byHash:  map[string][]byte{hCurrent: currentBytes},
	}

	plugin := data.NewPlugin(data.Config{Source: src})
	data.Register[Abilities](plugin)
	cardinal.RegisterPlugin(w, plugin)

	const missingHash = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	cardinal.RegisterSystem(w, func(state *manifestPreseedState) {
		_, ent := state.Manifest.Create()
		ent.Item.Set(component.ConfigManifest{Files: map[string]string{"testdata/abilities.json": missingHash}})
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(t, w)
	require.Panics(t, func() { tickOnce(t, w) })
}

// -------------------------------------------------------------------------------------------------
// Tests — error paths
// -------------------------------------------------------------------------------------------------

// TestPlugin_FetchErrorPanicsAtRegister verifies the Register-time fetch path: a Source error on
// the initial load surfaces as a panic from cardinal.RegisterPlugin, not as a deferred failure on
// the first tick.
func TestPlugin_FetchErrorPanicsAtRegister(t *testing.T) {
	w := newWorld(t)
	plugin := data.NewPlugin(data.Config{Source: errFake{}})
	data.Register[Abilities](plugin)
	require.Panics(t, func() { cardinal.RegisterPlugin(w, plugin) })
}

// TestPlugin_DuplicateNamePanics verifies Register[T] rejects two kinds sharing a Name().
func TestPlugin_DuplicateNamePanics(t *testing.T) {
	plugin := data.NewPlugin(data.Config{EmbeddedFS: testFS})
	data.Register[Abilities](plugin)
	require.Panics(t, func() { data.Register[abilitiesAlias](plugin) })
}

// abilitiesAlias collides with Abilities on Name() but uses a different JSONFile.
type abilitiesAlias struct {
	Items []AbilityRecord `json:"items"`
}

func (abilitiesAlias) Name() string     { return "test_abilities" }
func (abilitiesAlias) JSONFile() string { return "testdata/other.json" }

// TestPlugin_DuplicateJSONFilePanics verifies Register[T] rejects two kinds sharing a JSONFile().
func TestPlugin_DuplicateJSONFilePanics(t *testing.T) {
	plugin := data.NewPlugin(data.Config{EmbeddedFS: testFS})
	data.Register[Abilities](plugin)
	require.Panics(t, func() { data.Register[abilitiesFileTwin](plugin) })
}

// abilitiesFileTwin collides with Abilities on JSONFile() but uses a different Name.
type abilitiesFileTwin struct {
	Items []AbilityRecord `json:"items"`
}

func (abilitiesFileTwin) Name() string     { return "test_abilities_twin" }
func (abilitiesFileTwin) JSONFile() string { return "testdata/abilities.json" }

// -------------------------------------------------------------------------------------------------
// Tests — custom UnmarshalJSON / Resolver / Validator hooks
// -------------------------------------------------------------------------------------------------

// TestPlugin_CustomUnmarshalKind verifies a kind with its own UnmarshalJSON is honored (covers
// the path rampage's MapLevels uses).
func TestPlugin_CustomUnmarshalKind(t *testing.T) {
	bareArray := []byte(`["fireball","frostbolt"]`)
	src := &versionedFake{
		current: map[string]string{"custom.json": sha256hex(bareArray)},
		byHash:  map[string][]byte{sha256hex(bareArray): bareArray},
	}

	w := newWorld(t)
	plugin := data.NewPlugin(data.Config{Source: src})
	data.Register[customUnmarshalKind](plugin)
	cardinal.RegisterPlugin(w, plugin)

	require.Equal(t, []string{"fireball", "frostbolt"}, data.Get[customUnmarshalKind]().IDs)
}

// customUnmarshalKind has a custom UnmarshalJSON that accepts a bare JSON array of strings.
// Mixed receivers are intentional: UnmarshalJSON must be pointer (it mutates), while
// Name/JSONFile are value-receiver to satisfy the generic Definition constraint without
// forcing callers to write data.Register[*customUnmarshalKind].
//
//nolint:recvcheck // intentional pointer/value mix; see comment above.
type customUnmarshalKind struct {
	IDs []string
}

func (customUnmarshalKind) Name() string     { return "custom_unmarshal_kind" }
func (customUnmarshalKind) JSONFile() string { return "custom.json" }

func (c *customUnmarshalKind) UnmarshalJSON(b []byte) error {
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	c.IDs = arr
	return nil
}

// TestPlugin_ResolverUsesLocalNotPrimary verifies the documented Resolver policy: Resolver hooks
// always fetch through the local embedded filesystem, never the configured primary Source. The
// primary fake here errors on any file other than the kind's JSONFile() — a request for the
// side file would crash the test, proving the Resolver hook never went through primary.
func TestPlugin_ResolverUsesLocalNotPrimary(t *testing.T) {
	primaryJSON := []byte(`{"include":"testdata/resolver_side.json"}`)
	primarySrc := &mainOnlyFake{
		mainFile:  "resolver_main.json",
		mainBytes: primaryJSON,
	}

	w := newWorld(t)
	plugin := data.NewPlugin(data.Config{Source: primarySrc, EmbeddedFS: testFS})
	data.Register[resolverKind](plugin)
	cardinal.RegisterPlugin(w, plugin)

	got := data.Get[resolverKind]()
	require.Equal(t, "testdata/resolver_side.json", got.Include)
	require.Equal(t, "{\"extra\":\"hello\"}\n", got.Side)
}

// mainOnlyFake serves a single primary file and errors on anything else. Used to prove Resolver
// fetches don't go through the primary Source.
type mainOnlyFake struct {
	mainFile  string
	mainBytes []byte
}

func (f *mainOnlyFake) Fetch(_ context.Context, file, _ string) ([]byte, string, error) {
	if file != f.mainFile {
		return nil, "", errors.New("mainOnlyFake: primary should never be asked for: " + file)
	}
	return f.mainBytes, sha256hex(f.mainBytes), nil
}

// resolverKind exercises the Resolver hook by fetching a referenced second file in Resolve.
// Mixed receivers are intentional: Resolve must be pointer (it mutates), Name/JSONFile are
// value-receiver for the generic constraint.
//
//nolint:recvcheck // intentional pointer/value mix; see comment above.
type resolverKind struct {
	Include string `json:"include"`
	Side    string `json:"-"`
}

func (resolverKind) Name() string     { return "resolver_kind" }
func (resolverKind) JSONFile() string { return "resolver_main.json" }

func (r *resolverKind) Resolve(ctx context.Context, src data.Source) error {
	raw, _, err := src.Fetch(ctx, r.Include, "")
	if err != nil {
		return err
	}
	r.Side = string(raw)
	return nil
}

// TestPlugin_ValidatorRunsOnSuccessfulLoad verifies a Definition implementing Validator is loaded
// normally when Validate returns nil.
func TestPlugin_ValidatorRunsOnSuccessfulLoad(t *testing.T) {
	w := newWorld(t)

	plugin := data.NewPlugin(data.Config{EmbeddedFS: testFS})
	data.Register[validatedAbilities](plugin)
	cardinal.RegisterPlugin(w, plugin)

	require.Len(t, data.Get[validatedAbilities]().Items, 2)
}

// validatedAbilities accepts any non-empty Items slice. Used to verify Validate is invoked.
type validatedAbilities struct {
	Items []AbilityRecord `json:"items"`
}

func (validatedAbilities) Name() string     { return "test_validated_abilities" }
func (validatedAbilities) JSONFile() string { return "testdata/abilities.json" }

func (v validatedAbilities) Validate() error {
	if len(v.Items) == 0 {
		return errors.New("validated_abilities: empty items")
	}
	return nil
}

// TestPlugin_ValidateErrorPanicsAtRegister verifies a Validator returning an error surfaces as a
// panic from cardinal.RegisterPlugin — same fail-loud discipline as a fetch or unmarshal error.
func TestPlugin_ValidateErrorPanicsAtRegister(t *testing.T) {
	w := newWorld(t)
	plugin := data.NewPlugin(data.Config{EmbeddedFS: testFS})
	data.Register[rejectingAbilities](plugin)
	require.Panics(t, func() { cardinal.RegisterPlugin(w, plugin) })
}

// rejectingAbilities always fails Validate.
type rejectingAbilities struct {
	Items []AbilityRecord `json:"items"`
}

func (rejectingAbilities) Name() string     { return "test_rejecting_abilities" }
func (rejectingAbilities) JSONFile() string { return "testdata/abilities.json" }

func (rejectingAbilities) Validate() error { return errors.New("rejecting_abilities: rejected") }

// TestPlugin_PointerReceiverValidatorRuns verifies a Validator implemented on a POINTER receiver
// is still detected and run. MakeAssemble asserts the Validator interface against &def (the
// pointer), not def (the value): a pointer-receiver Validate is not in the value's method set, so
// a value assertion would silently skip it and let invalid config load. rejectingAbilities above
// uses a value receiver and so cannot catch that regression — this kind does.
func TestPlugin_PointerReceiverValidatorRuns(t *testing.T) {
	w := newWorld(t)
	plugin := data.NewPlugin(data.Config{EmbeddedFS: testFS})
	data.Register[pointerRejectingKind](plugin)
	require.Panics(t, func() { cardinal.RegisterPlugin(w, plugin) })
}

// pointerRejectingKind implements Validator on a pointer receiver and always rejects.
//
//nolint:recvcheck // intentional pointer/value receiver mix; pointer-receiver Validate is the point.
type pointerRejectingKind struct {
	Items []AbilityRecord `json:"items"`
}

func (pointerRejectingKind) Name() string     { return "test_pointer_rejecting_kind" }
func (pointerRejectingKind) JSONFile() string { return "testdata/abilities.json" }

func (k *pointerRejectingKind) Validate() error {
	return errors.New("pointer_rejecting_kind: rejected")
}
