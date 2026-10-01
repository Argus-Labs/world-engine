package system

import (
	"math"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	"github.com/stretchr/testify/require"
)

// These tests pin the pipeline-side wiring that the bug left dead: PhysicsPipelineSystem.Run must
// arm NoPersistedActiveContactsBaseline when ensurePhysicsSingleton just created the singleton,
// so the suppressed contact flush takes the adoptLiveContactsWithoutEmit branch. They drive the real
// Init + PreUpdate systems through a headless Cardinal world (the same reflection init shim the e2e
// harness uses) and the test owns the Runtime so it can inspect NoPersistedActiveContactsBaseline and
// ActiveContacts directly. The contact-flush contract itself is pinned in
// internal/no_baseline_internal_test.go.

// boxShape builds a non-sensor box collider for the baseline test fixture.
func boxShape(hw, hh float64) immutable.Slice[component.ColliderShape] {
	return immutable.SliceOf(component.ColliderShape{
		ShapeType:    component.ShapeTypeBox,
		HalfExtents:  component.Vec2{X: hw, Y: hh},
		Density:      1,
		Friction:     0.6,
		CategoryBits: 1,
		MaskBits:     ^uint64(0),
	})
}

// setupBodies spawns the overlapping ground + crate pair on Init so InitPhysicsSystem's
// FullRebuildFromECS sees them. The crate is embedded in the ground enough that one step cannot
// separate them, so the suppressed flush always has exactly one live contact.
type setupBodies struct{}

func (setupBodies) Run(w *cardinal.World) {
	bodies := w.Exact[physicsBodyRow]()
	ground := bodies.Create()
	ground.Set(component.Transform2D{Position: component.Vec2{Y: -1}})
	ground.Set(component.Velocity2D{})
	ground.Set(component.PhysicsBody2D{
		BodyType:        component.BodyTypeStatic,
		Active:          true,
		Awake:           true,
		SleepingAllowed: true,
		GravityScale:    1,
		Shapes:          boxShape(40, 1),
	})
	crate := bodies.Create()
	crate.Set(component.Transform2D{Position: component.Vec2{Y: -0.45}})
	crate.Set(component.Velocity2D{})
	crate.Set(component.PhysicsBody2D{
		BodyType:        component.BodyTypeDynamic,
		Active:          true,
		Awake:           true,
		SleepingAllowed: true,
		GravityScale:    1,
		Shapes:          boxShape(0.5, 0.5),
	})
}

// removeSingletonOnce destroys the physics singleton on its first PreUpdate run, simulating a
// cross-plugin/migration restore whose snapshot did not carry the physics singleton entity. It
// must run before PhysicsPipelineSystem so the pipeline's ensurePhysicsSingleton re-creates it
// and reports created=true.
type removeSingletonOnce struct {
	done *bool
}

func (r removeSingletonOnce) Run(w *cardinal.World) {
	if *r.done {
		return
	}
	*r.done = true
	row, err := w.Exact[physicsSingletonRow]().Iter().Single()
	if err != nil {
		// No singleton to remove (or duplicate, which ensurePhysicsSingleton would panic on);
		// either way the pipeline below behaves as it would have. Marking done keeps this off
		// for the rest of the run.
		return
	}
	row.Destroy()
}

// countContactBegins records the number of ContactBegin system events emitted this tick into the
// test-owned counter. System events are cleared at the end of w.world.Tick, so the count must be
// read from inside a system (here, Update, after the PreUpdate pipeline emitted).
type countContactBegins struct {
	count *int
}

func (c countContactBegins) Run(w *cardinal.World) {
	n := 0
	for range w.SystemEvents[event.ContactBeginEvent]() {
		n++
	}
	*c.count = n
}

// probeSingleton calls ensurePhysicsSingleton twice on Init and records whether each call created
// the singleton, plus how many singleton entities exist after. This pins the created-signal the
// pipeline fix arms NoPersistedActiveContactsBaseline from.
type probeSingleton struct {
	first  *bool
	second *bool
	count  *int
}

func (p probeSingleton) Run(w *cardinal.World) {
	s := w.Exact[physicsSingletonRow]()
	*p.first = ensurePhysicsSingleton(s)
	n := 0
	for range s.Iter() {
		n++
	}
	*p.count = n
	*p.second = ensurePhysicsSingleton(s)
}

// newBaselineTestWorld builds a headless Cardinal world with the physics components and the four
// contact system events registered, but no systems: callers register the systems they need. The
// test owns the Runtime (constructed separately) so it can inspect Runtime fields directly.
func newBaselineTestWorld(t *testing.T) *cardinal.World {
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
	w.RegisterComponent[component.Transform2D]()
	w.RegisterComponent[component.Velocity2D]()
	w.RegisterComponent[component.PhysicsBody2D]()
	w.RegisterComponent[component.PhysicsSingletonTag]()
	w.RegisterComponent[component.ActiveContacts]()
	w.RegisterSystemEvent[event.ContactBeginEvent]()
	w.RegisterSystemEvent[event.ContactEndEvent]()
	w.RegisterSystemEvent[event.TriggerBeginEvent]()
	w.RegisterSystemEvent[event.TriggerEndEvent]()
	return w
}

// initECS runs the world's Init-hook systems without StartGame (which would stand up NATS and the
// service loop). This is the same reflection shim the e2e harness uses; cardinal.World.init is
// unexported and also starts the snapshot loop, so the tests reach the inner ecs.World.Init here.
func initECS(t *testing.T, w *cardinal.World) {
	t.Helper()
	v := reflect.ValueOf(w).Elem()
	f := v.FieldByName("world")
	require.True(t, f.IsValid(), "cardinal.World: missing embedded ecs world field")
	inner := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	m := inner.MethodByName("Init")
	require.True(t, m.IsValid(), "ecs.World: missing Init method")
	m.Call(nil)
}

// runBaselineScenario wires the Init/PreUpdate/Update systems around a Runtime the test owns,
// runs Init + one tick, and returns the Runtime and the number of ContactBegin events emitted on
// that tick. When crossPluginRestore is true, a PreUpdate system destroys the singleton before the
// pipeline runs, simulating a snapshot restore that omitted the physics singleton.
func runBaselineScenario(t *testing.T, crossPluginRestore bool) (*internal.Runtime, int) {
	t.Helper()
	w := newBaselineTestWorld(t)

	// Init: spawn bodies first, then the plugin's InitPhysicsSystem (creates singleton + Box2D world).
	rt := internal.NewRuntime(component.Vec2{Y: -10}, 1.0/60.0, 4, 0)
	w.RegisterSystem(setupBodies{}, cardinal.WithHook(cardinal.Init))
	w.RegisterSystem(NewInitPhysicsSystem(rt), cardinal.WithHook(cardinal.Init))

	var destroyed bool
	if crossPluginRestore {
		// PreUpdate, before the pipeline: drop the singleton once to model a restore that did
		// not carry it.
		w.RegisterSystem(removeSingletonOnce{done: &destroyed}, cardinal.WithHook(cardinal.PreUpdate))
	}
	w.RegisterSystem(NewPhysicsPipelineSystem(rt), cardinal.WithHook(cardinal.PreUpdate))

	var begins int
	w.RegisterSystem(countContactBegins{count: &begins}, cardinal.WithHook(cardinal.Update))

	initECS(t, w) // runs setupBodies + InitPhysicsSystem; world exists, SuppressContactsStep=true.
	w.Tick(time.Unix(0, 0))

	if crossPluginRestore {
		require.True(t, destroyed, "singleton was not destroyed before the pipeline ran")
	}
	return rt, begins
}

// TestEnsurePhysicsSingletonReportsCreated pins the return value the fix depends on: the first call
// on an empty world creates the singleton and reports true; the immediate second call finds it and
// reports false.
func TestEnsurePhysicsSingletonReportsCreated(t *testing.T) {
	t.Parallel()
	w := newBaselineTestWorld(t)
	var first, second bool
	var count int
	w.RegisterSystem(probeSingleton{first: &first, second: &second, count: &count},
		cardinal.WithHook(cardinal.Init))
	initECS(t, w)

	require.True(t, first,
		"ensurePhysicsSingleton must report created=true when the singleton is absent")
	require.False(t, second,
		"ensurePhysicsSingleton must report created=false when the singleton already exists")
	require.Equal(t, 1, count,
		"exactly one physics singleton entity must exist after ensurePhysicsSingleton")
}

// TestPipelineArmsNoBaselineWhenSingletonCreatedMidRun is the end-to-end guard for the fix. After
// a cross-plugin restore that omits the physics singleton, the pipeline must re-create it, arm
// NoPersistedActiveContactsBaseline, and the suppressed flush must adopt the live contact silently
// — zero ContactBegin events — instead of firing a Begin for every overlap. Before the fix the
// flag could never arm, so this path was dead code and the diff path would have emitted a Begin.
func TestPipelineArmsNoBaselineWhenSingletonCreatedMidRun(t *testing.T) {
	t.Parallel()
	rt, begins := runBaselineScenario(t, true)

	require.Equal(t, 0, begins,
		"no ContactBegin may fire when the singleton is created mid-run (no persisted baseline); "+
			"the suppressed flush must adopt live contacts silently")
	require.False(t, rt.NoPersistedActiveContactsBaseline,
		"the no-persisted-baseline flag must be cleared after the suppressed flush consumes it")
	require.Len(t, rt.ActiveContacts, 1,
		"the touching pair must be adopted into the in-memory contact map without emitting")
}

// TestPipelineEmitsBeginsWhenSingletonPresent is the no-regression companion: with the singleton
// present (a normal fresh world whose Init created it), the pipeline must NOT arm the flag and the
// suppressed flush must diff against the (empty) persisted baseline and emit a Begin for the
// touching pair. The contrast with the cross-plugin case above is what proves the fix took the
// right branch.
func TestPipelineEmitsBeginsWhenSingletonPresent(t *testing.T) {
	t.Parallel()
	rt, begins := runBaselineScenario(t, false)

	require.Equal(t, 1, begins,
		"the suppressed diff must emit exactly one ContactBegin for the touching pair when a "+
			"baseline singleton exists (fresh world)")
	require.False(t, rt.NoPersistedActiveContactsBaseline,
		"the no-persisted-baseline flag must never arm when the singleton is present")
	require.Len(t, rt.ActiveContacts, 1,
		"the touching pair must be recorded in the in-memory contact map")
}
