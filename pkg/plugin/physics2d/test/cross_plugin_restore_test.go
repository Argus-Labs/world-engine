package physics2d_test

import (
	"math"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Cross-plugin restore integration test: a snapshot produced by a world that did NOT run the
// physics2d plugin (so it never created the physics singleton), restored on a world that DOES run
// it. This is the real-world trigger for the bug: World B's init creates the singleton, FromProto
// replaces all ECS state with World A's entities (which omit the singleton), and the first pipeline
// tick must re-create it and arm NoPersistedActiveContactsBaseline so the suppressed flush adopts
// the existing overlaps silently — zero spurious ContactBegin events — instead of firing one per
// overlap. The in-package system test (pipeline_singleton_baseline_internal_test.go) simulates the
// "singleton missing at pipeline time" half by destroying the singleton mid-run; this test adds the
// real snapshot encode/decode path that leaves the singleton absent, and drives the full PreUpdate
// pipeline through a Cardinal Tick.

// crossPluginSingletonArchetype mirrors the plugin's singleton archetype so an external test can
// read the singleton's ActiveContacts after the tick. Field names/types match component.Component.
type crossPluginSingletonArchetype struct {
	Tag            physics.PhysicsSingletonTag
	ActiveContacts physics.ActiveContacts
}

// crossPluginBodyArchetype is the archetype the spawn system writes ground+crate into on World A.
type crossPluginBodyArchetype struct {
	Tag harnessTag
	T   physics.Transform2D
	V   physics.Velocity2D
	PB  physics.PhysicsBody2D
}

// cpSpawnBodies spawns an overlapping static ground and dynamic crate on Init. The crate is embedded
// in the ground enough that one step cannot separate them, so the first suppressed flush always has
// exactly one live contact. World A runs this without the physics plugin, so the bodies sit in ECS
// unsimulated and NO singleton is created.
type cpSpawnBodies struct{}

func (cpSpawnBodies) Run(w *cardinal.World) {
	bodies := w.Exact[crossPluginBodyArchetype]()
	ground := bodies.Create()
	ground.Set(harnessTag{Role: "ground"})
	ground.Set(physics.Transform2D{Position: physics.Vec2{Y: -1}})
	ground.Set(physics.Velocity2D{})
	ground.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(40, 1)...))
	crate := bodies.Create()
	crate.Set(harnessTag{Role: "crate"})
	crate.Set(physics.Transform2D{Position: physics.Vec2{Y: -0.45}})
	crate.Set(physics.Velocity2D{})
	crate.Set(newRigid(physics.BodyTypeDynamic, boxColliderShapes(0.5, 0.5)...))
}

// cpCountBegins records the number of ContactBegin system events emitted this tick. Registered on
// World B's Update hook so it reads events the PreUpdate pipeline emitted before they are cleared
// at the end of the tick.
type cpCountBegins struct {
	count *int
}

func (c cpCountBegins) Run(w *cardinal.World) {
	n := 0
	for range w.SystemEvents[physics.ContactBeginEvent]() {
		n++
	}
	*c.count = n
}

// cpNewWorld builds a headless Cardinal world with the physics components registered in the
// canonical order (physics.RegisterComponents first) so component IDs match across World A and
// World B even though only World B registers the plugin. WithPlugin controls whether the physics2d
// plugin (and thus its systems and system events) is registered.
func cpNewWorld(t *testing.T, withPlugin bool) (*cardinal.World, *physics.Plugin) {
	t.Helper()
	debug := false
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		Region:              "local",
		Organization:        "physics-test",
		Project:             "physics-test",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        math.MaxUint32,
		Debug:               &debug,
	})
	require.NoError(t, err)
	// Register the physics components in the same order on both worlds so snapshot component IDs
	// line up; the harness relies on this same invariant for its same-plugin restore.
	physics.RegisterComponents(w)
	w.RegisterComponent[harnessTag]()
	var p *physics.Plugin
	if withPlugin {
		p = physics.NewPlugin(physics.Config{Gravity: physics.Vec2{Y: -10}, TickRate: 60})
		w.RegisterPlugin(p) // re-registers components (idempotent), registers system events + systems.
	}
	return w, p
}

// cpEncodeState reaches cardinal.World's embedded ecs.World.EncodeState via reflection (the same
// escape hatch worldStateProto and initCardinalECS use; physics2d_test cannot import ecs/internal)
// and returns the snapshot as a decoded *cardinalv1.WorldState ready for FromProto.
func cpEncodeState(t *testing.T, w *cardinal.World) *cardinalv1.WorldState {
	t.Helper()
	v := reflect.ValueOf(w).Elem()
	f := v.FieldByName("world")
	require.True(t, f.IsValid(), "cardinal.World: missing embedded ecs world field")
	inner := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	m := inner.MethodByName("EncodeState")
	require.True(t, m.IsValid(), "ecs.World: missing EncodeState method")
	out := m.Call([]reflect.Value{reflect.ValueOf([]byte(nil))})
	data, ok := out[0].Interface().([]byte)
	require.True(t, ok, "EncodeState returned %T, want []byte", out[0].Interface())
	ws := &cardinalv1.WorldState{}
	require.NoError(t, proto.Unmarshal(data, ws), "decode world state")
	return ws
}

// cpRestoreWorld reaches ecs.World.FromProto via reflection and loads the snapshot into w,
// reproducing Cardinal's restore ordering (Init first, then FromProto throws the init state away).
func cpRestoreWorld(t *testing.T, w *cardinal.World, state *cardinalv1.WorldState) {
	t.Helper()
	v := reflect.ValueOf(w).Elem()
	f := v.FieldByName("world")
	require.True(t, f.IsValid(), "cardinal.World: missing embedded ecs world field")
	inner := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	m := inner.MethodByName("FromProto")
	require.True(t, m.IsValid(), "ecs.World: missing FromProto method")
	out := m.Call([]reflect.Value{reflect.ValueOf(state)})
	if err, ok := out[0].Interface().(error); ok && err != nil {
		t.Fatalf("FromProto: %v", err)
	}
}

// TestCrossPluginRestoreArmsNoBaseline is the end-to-end cross-plugin restore scenario. World A
// (no physics plugin) holds an overlapping ground+crate but never created the physics singleton;
// its snapshot therefore omits it. World B (with the plugin) inits — creating the singleton — then
// restores World A's snapshot, which replaces all ECS state so the singleton is gone again. The
// first pipeline tick must re-create the singleton (created=true), arm NoPersistedActiveContactsBaseline,
// and the suppressed flush must adopt the live overlap silently: exactly 0 ContactBegin events, and
// the adopted pair lands in the new singleton's ActiveContacts. Before the fix, the flag could never
// arm, so this path would have fired a ContactBegin for the overlap.
func TestCrossPluginRestoreArmsNoBaseline(t *testing.T) {
	t.Parallel()

	// --- World A: no physics plugin, just components + bodies. No singleton is ever created. ---
	worldA, _ := cpNewWorld(t, false)
	worldA.RegisterSystem(cpSpawnBodies{}, cardinal.WithHook(cardinal.Init))
	initCardinalECS(worldA) // runs cpSpawnBodies; ground+crate exist, singleton absent.
	snapshotState := cpEncodeState(t, worldA)

	// Sanity: World A must have the two bodies and NO physics singleton.
	singletonA := 0
	for range worldA.Exact[crossPluginSingletonArchetype]().Iter() {
		singletonA++
	}
	require.Zero(t, singletonA, "World A must not have created a physics singleton (no plugin)")
	bodyA := 0
	for range worldA.Exact[crossPluginBodyArchetype]().Iter() {
		bodyA++
	}
	require.Equal(t, 2, bodyA, "World A must have spawned exactly the ground and crate")

	// --- World B: full physics plugin. Init creates the singleton + empty Box2D world. ---
	worldB, plugin := cpNewWorld(t, true)
	initCardinalECS(worldB) // runs InitPhysicsSystem: ensurePhysicsSingleton + empty FullRebuildFromECS.
	// Sanity: after init, World B has exactly one singleton.
	singletonB := 0
	for range worldB.Exact[crossPluginSingletonArchetype]().Iter() {
		singletonB++
	}
	require.Equal(t, 1, singletonB, "World B init must create exactly one physics singleton")

	// --- Restore World A's snapshot into World B. FromProto replaces all ECS state, so the
	// init-created singleton is gone and only the ground+crate remain. ---
	cpRestoreWorld(t, worldB, snapshotState)
	singletonAfterRestore := 0
	for range worldB.Exact[crossPluginSingletonArchetype]().Iter() {
		singletonAfterRestore++
	}
	require.Zero(t, singletonAfterRestore,
		"after the cross-plugin restore the singleton must be absent (snapshot omitted it)")

	// --- First pipeline tick. Register a count system on Update to read ContactBegin events the
	// PreUpdate pipeline emitted before the tick clears them. ---
	var begins int
	worldB.RegisterSystem(cpCountBegins{count: &begins}, cardinal.WithHook(cardinal.Update))
	worldB.Tick(time.Unix(0, 0))

	require.Equal(t, 0, begins,
		"the cross-plugin restore must arm NoPersistedActiveContactsBaseline so the suppressed "+
			"flush adopts the existing overlap silently — zero ContactBegin events")

	// The pipeline re-created the singleton during that PreUpdate; its ActiveContacts must now hold
	// the adopted pair (proving adoptLiveContactsWithoutEmit ran and the pipeline persisted it).
	singletonFinal := 0
	var contacts physics.ActiveContacts
	for row := range worldB.Exact[crossPluginSingletonArchetype]().Iter() {
		singletonFinal++
		contacts = row.Get[physics.ActiveContacts]()
	}
	require.Equal(t, 1, singletonFinal, "the pipeline must have re-created exactly one singleton")
	require.Equal(t, 1, contacts.Pairs.Len(),
		"the adopted overlap must be persisted to the new singleton's ActiveContacts; got %d pairs",
		contacts.Pairs.Len())

	// The Box2D world must be alive with both bodies reconciled in.
	require.NotNil(t, plugin.Engine(), "Plugin.Engine() must be alive after the restore tick")
	groundBodies := 0
	for range worldB.Exact[crossPluginBodyArchetype]().Iter() {
		groundBodies++
	}
	require.Equal(t, 2, groundBodies, "the restored bodies must remain in ECS after the tick")
}
