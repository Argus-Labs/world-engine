package system

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/data/component"
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
)

// State owns the data plugin's per-instance load state: registered kinds, the in-memory catalog,
// and the manifest of per-file hashes currently reflected in that catalog.
//
// data.Plugin owns one State per world and wires it into the ReconcileSystem it registers.
// Splitting it out keeps the load-and-reconcile logic colocated with the components it operates
// on, while plugin.go holds the registration lifecycle (kinds, world binding, guards).
type State struct {
	loaders map[string]kindLoader // jsonFile → loader
	catalog map[string]Definition // Name() → loaded value
	// manifest is the hash currently reflected in the catalog, per jsonFile, sorted by path. It is
	// the same type the snapshot holds, so the reconcile pass compares and writes it directly.
	manifest component.ConfigManifest
}

// NewState builds an empty State. Call AddKind for each registered kind before LoadAll.
func NewState() *State {
	return &State{
		loaders: map[string]kindLoader{},
		catalog: map[string]Definition{},
	}
}

// AssembleFunc materializes raw bytes into a fully-loaded Definition: json.Unmarshal → Resolve (if
// implemented) → Validate (if implemented). Used both at initial load and at reconcile re-fetches
// so a snapshot-pinned version goes through the same hooks as a fresh load.
//
// resolverSource is the Source the Resolver hook (if any) uses to fetch additional files. By
// convention this is always the local embed (never the operator), because Resolver-fetched files
// are heavy, designer-bundled, build-time assets that don't fit a "designer edits a row in a web
// UI" workflow.
type AssembleFunc func(ctx context.Context, resolverSource Source, raw []byte) (Definition, error)

// kindLoader is one entry in State.loaders, keyed by jsonFile: enough to assemble bytes into a
// loaded Definition and key the result by Name().
type kindLoader struct {
	name     string
	assemble AssembleFunc
}

// hook reports whether T implements H, checking def first then &def: def covers a pointer-type T
// (where &def is **T), &def covers a value-type T with pointer-receiver methods.
func hook[H, T any](def *T) (H, bool) {
	if h, ok := any(*def).(H); ok {
		return h, true
	}
	h, ok := any(def).(H)
	return h, ok
}

// MakeAssemble returns the standard assemble function for kind T: json.Unmarshal into a fresh T,
// then Resolve and Validate if T implements them. Errors from any step propagate to the caller
// (LoadAll and Reconcile both panic on them).
func MakeAssemble[T Definition]() AssembleFunc {
	return func(ctx context.Context, resolverSource Source, raw []byte) (Definition, error) {
		var def T
		if err := json.Unmarshal(raw, &def); err != nil {
			return nil, err
		}
		if r, ok := hook[Resolver](&def); ok {
			if err := r.Resolve(ctx, resolverSource); err != nil {
				return nil, err
			}
		}
		if v, ok := hook[Validator](&def); ok {
			if err := v.Validate(); err != nil {
				return nil, err
			}
		}
		return def, nil
	}
}

// AddKind enrolls a kind into the catalog. Panics on duplicate Name or JSONFile — both are wiring
// bugs that would otherwise silently collide.
func (s *State) AddKind(name, jsonFile string, assemble AssembleFunc) {
	if existing, ok := s.loaders[jsonFile]; ok {
		panic(eris.Errorf("data: JSON file %q already registered (by kind %q)", jsonFile, existing.name))
	}
	for _, l := range s.loaders {
		if l.name == name {
			panic(eris.Errorf("data: kind name %q already registered", name))
		}
	}
	s.loaders[jsonFile] = kindLoader{name: name, assemble: assemble}
}

// Get looks up a kind in the catalog by name. Returns (value, true) on hit; (nil, false) on miss.
// The generic data.Get[T] wraps this and panics on miss with a typed message.
func (s *State) Get(name string) (Definition, bool) {
	v, ok := s.catalog[name]
	return v, ok
}

// MustGet returns the loaded value for the kind keyed by name, panicking with a clear message if
// it was never registered. Used as the implementation of data.Get[T].
func (s *State) MustGet(name string) Definition {
	v, ok := s.catalog[name]
	if !ok {
		panic(fmt.Sprintf("data: kind %q not registered with this plugin", name))
	}
	return v
}

// LoadAll fetches every registered kind's JSON via primary, assembles it (handing resolverSource
// to any Resolver hook), and stores the result in the catalog. Records the per-file manifest
// hashes the primary source reported. Panics on any error — startup-time loading failures should
// be loud and immediate.
//
// primary is the data source for each kind's JSONFile() (embed in dev, operator in prod).
// resolverSource is what Resolver hooks fetch additional files through — always the local embed,
// never the operator (see AssembleFunc doc).
//
// Iterates jsonFile in sorted order: boot-time log output is reproducible across runs, and the
// manifest comes out in its canonical order.
func (s *State) LoadAll(ctx context.Context, primary, resolverSource Source) {
	entries := make([]component.ConfigFileHash, 0, len(s.loaders))
	for _, file := range slices.Sorted(maps.Keys(s.loaders)) {
		l := s.loaders[file]
		raw, gotHash, err := primary.Fetch(ctx, file, "")
		if err != nil {
			panic(eris.Wrapf(err, "data: fetching %q", file))
		}
		def, err := l.assemble(ctx, resolverSource, raw)
		if err != nil {
			panic(eris.Wrapf(err, "data: loading %q", file))
		}
		s.catalog[l.name] = def
		entries = append(entries, component.ConfigFileHash{Path: file, Hash: gotHash})
	}
	s.manifest = component.ConfigManifest{Files: immutable.SliceOf(entries...)}
}

// ReconcileSystem keeps the catalog matched to the world's configuration manifest.
type ReconcileSystem struct {
	Catalog        *State
	Primary        Source
	ResolverSource Source
}

func (rs *ReconcileSystem) Run(w *cardinal.World) {
	rs.Catalog.Reconcile(w, rs.Primary, rs.ResolverSource)
}

// manifestRow is the exact archetype of the ConfigManifest singleton entity.
type manifestRow struct {
	Item component.ConfigManifest
}

// Reconcile is the data plugin's per-tick reconcile pass. Runs every PreUpdate (cardinal's
// earliest tick hook), fast-paths the steady state, and on a post-restore mismatch attempts to
// re-fetch each changed file at the snapshot's hash so the world keeps running on the config it
// was running before the restart.
//
// Lifecycle coverage with this one system:
//   - Fresh boot: ErrSingleNoResult → create ConfigManifest from s.manifest.
//   - Restart, same config: manifests match → no-op (one search + one allocation-free compare).
//   - Restart, restored snapshot whose config differs: re-fetch each changed file at the
//     snapshot's hash. Source can deliver → swap catalog atomically. Source errors (versioned
//     source lost a version) → panic. Source returns wrong-hash content (single-version source,
//     i.e. embed → rebuilt binary) → log a warning, keep current config, rewrite the component to
//     s.manifest so the next snapshot is self-consistent and the warning doesn't re-fire.
//   - cardinal.Reset()+Init() debug reload: wipes entities but Plugin.Register is not re-run, so
//     the catalog persists. Next tick's reconcile sees ErrSingleNoResult and re-creates the
//     ConfigManifest from s.manifest.
//   - Restore from a pre-feature snapshot (no ConfigManifest): ErrSingleNoResult → create.
//
// All-or-nothing: a versioned source delivers every needed historical file or it panics; a
// single-version source bails to the warn path on the first hash that doesn't match. No
// half-old/half-new catalog state.
//
// primary is the data source for each kind's JSONFile() re-fetch at the snapshot's hash.
// resolverSource is what Resolver hooks fetch additional files through (always local embed).
func (s *State) Reconcile(w *cardinal.World, primary, resolverSource Source) {
	manifest := w.Exact[manifestRow]()
	ent, err := manifest.Iter().Single()
	switch {
	case errors.Is(err, cardinal.ErrSingleNoResult):
		ent = manifest.Create()
		ent.Set(s.manifest)
		return
	case errors.Is(err, cardinal.ErrSingleMultipleResult):
		panic(eris.New("data: more than one config-manifest singleton"))
	case err != nil:
		panic(eris.Wrap(err, "data: querying config-manifest singleton"))
	}

	// Steady state: nothing to do. The compare is order-sensitive on purpose. A snapshot written
	// before this component held an ordered list restores in arbitrary order, so it reads as changed
	// here, finds every hash already matching below, and reaches the rewrite at the bottom, which
	// stores it sorted. Every tick after that takes this return.
	snap := ent.Get[component.ConfigManifest]()
	if immutable.Equal(snap.Files, s.manifest.Files) {
		return
	}

	// A duplicate path would load the last entry's bytes but record the first entry's hash, leaving
	// the catalog and the manifest on different versions. This system only ever writes each path once.
	seen := make(map[string]struct{}, snap.Files.Len())
	for e := range snap.Files.Values() {
		if _, dup := seen[e.Path]; dup {
			panic(eris.Errorf("data: config-manifest lists %q more than once", e.Path))
		}
		seen[e.Path] = struct{}{}
	}

	ctx := context.Background()
	temp := map[string]Definition{}
	for e := range snap.Files.Values() {
		if cur, ok := s.manifest.Hash(e.Path); ok && cur == e.Hash {
			continue
		}
		loader, owned := s.loaders[e.Path]
		if !owned {
			continue
		}
		raw, gotHash, fetchErr := primary.Fetch(ctx, e.Path, e.Hash)
		if fetchErr != nil {
			panic(eris.Wrapf(fetchErr, "data: source cannot serve %q at hash %s required by snapshot", e.Path, e.Hash))
		}
		if gotHash != e.Hash {
			w.Logger().Warn().
				Interface("snapshot", snap).
				Interface("current", s.manifest).
				Msg("data: config changed since snapshot; resuming on current config")
			ent.Set(s.manifest)
			return
		}
		def, assembleErr := loader.assemble(ctx, resolverSource, raw)
		if assembleErr != nil {
			panic(eris.Wrapf(assembleErr, "data: loading %q at hash %s", e.Path, e.Hash))
		}
		temp[loader.name] = def
	}

	maps.Copy(s.catalog, temp)
	// Adopt the snapshot's hash for every file this instance owns. A file it does not own is not
	// its business and stays out. Rebuilt in path order, so the component written from here on is
	// canonical even when the snapshot it came from was not.
	entries := make([]component.ConfigFileHash, 0, len(s.loaders))
	for _, file := range slices.Sorted(maps.Keys(s.loaders)) {
		hash, _ := s.manifest.Hash(file)
		if snapHash, ok := snap.Hash(file); ok {
			hash = snapHash
		}
		entries = append(entries, component.ConfigFileHash{Path: file, Hash: hash})
	}
	s.manifest = component.ConfigManifest{Files: immutable.SliceOf(entries...)}
	ent.Set(s.manifest)
}
