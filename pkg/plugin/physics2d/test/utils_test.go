package physics2d_test

import (
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	phycomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/stretchr/testify/require"
)

// makeWorld creates a Cardinal world at 60 Hz with the physics plugin installed, returning the
// world and the registered plugin instance (queries and Reset are methods on it).
// World gravity is set from gravity; plugin tick rate matches the world tick rate.
// Workers stays at the serial default (0); use makeWorldWorkers for a parallel world.
func makeWorld(t *testing.T, gravity physics.Vec2) (*cardinal.World, *physics.Plugin) {
	t.Helper()
	return makeWorldWorkers(t, gravity, 0)
}

// makeWorldWorkers is makeWorld with an explicit physics.Config.Workers value
// (box2d.WorldDef.WorkerCount; results are byte-identical for every value).
func makeWorldWorkers(t *testing.T, gravity physics.Vec2, workers int) (*cardinal.World, *physics.Plugin) {
	t.Helper()
	debug := true
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		Region:              "local",
		Organization:        "wb-test",
		Project:             "wb-test",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1_000_000,
		Debug:               &debug,
	})
	require.NoError(t, err)
	p := physics.NewPlugin(physics.Config{
		Gravity:  gravity,
		TickRate: 60,
		Workers:  workers,
	})
	cardinal.RegisterPlugin(w, p)
	return w, p
}

// newRigid returns a PhysicsBody2D with Active/Awake/SleepingAllowed true and GravityScale 1.
func newRigid(bodyType physics.BodyType, shapes ...physics.ShapeSlot) physics.PhysicsBody2D {
	return phycomp.NewPhysicsBody2D(bodyType, shapes...)
}

// newRigidNoGravity is like newRigid but GravityScale 0 (e.g. zero-gravity scene bodies).
func newRigidNoGravity(bodyType physics.BodyType, shapes ...physics.ShapeSlot) physics.PhysicsBody2D {
	r := phycomp.NewPhysicsBody2D(bodyType, shapes...)
	r.GravityScale = 0
	return r
}

// spawnState is the system state test spawners use: the body archetype and one shape search
// per geometry kind. Cardinal wires only top-level fields, so every spawning system lists
// these itself (or uses this type).
type spawnState struct {
	cardinal.BaseSystemState
	Spawn    spawnArchetype
	Circles  physics.CircleShapes
	Boxes    physics.BoxShapes
	Polygons physics.PolygonShapes
	Chains   physics.ChainShapes
	Edges    physics.EdgeShapes
	Capsules physics.CapsuleShapes
}

// mustSpawn unwraps a Spawn call. Every shape in these tests is a literal, so a rejection is
// a typo in the test rather than something a caller could handle.
func mustSpawn(slot physics.ShapeSlot, err error) physics.ShapeSlot {
	if err != nil {
		panic(err)
	}
	return slot
}

// spawnShape spawns def through the search matching its geometry kind and returns the slot.
func spawnShape[G physics.Geometry](s *spawnState, def physics.ShapeDef[G]) physics.ShapeSlot {
	switch d := any(def).(type) {
	case physics.ShapeDef[physics.CircleGeom]:
		return mustSpawn(d.Spawn(&s.Circles))
	case physics.ShapeDef[physics.BoxGeom]:
		return mustSpawn(d.Spawn(&s.Boxes))
	case physics.ShapeDef[physics.PolygonGeom]:
		return mustSpawn(d.Spawn(&s.Polygons))
	case physics.ShapeDef[physics.ChainGeom]:
		return mustSpawn(d.Spawn(&s.Chains))
	case physics.ShapeDef[physics.EdgeGeom]:
		return mustSpawn(d.Spawn(&s.Edges))
	case physics.ShapeDef[physics.CapsuleGeom]:
		return mustSpawn(d.Spawn(&s.Capsules))
	}
	panic("spawnShape: unknown geometry kind")
}

func tickN(t *testing.T, w *cardinal.World, n int) {
	t.Helper()
	for i := range n {
		w.Tick(time.Unix(int64(i), 0))
		if t.Failed() {
			t.Fatalf("failed at tick %d", i)
		}
	}
}

// circleSlot spawns the stock test circle (radius 0.5) and returns its slot.
func circleSlot(s *spawnState) physics.ShapeSlot {
	return spawnShape(s, physics.Circle(0.5).Material(0.3, 0, 1).Filter(0xFFFF, 0xFFFF))
}

// boxSlot spawns a stock test box with the given half extents and returns its slot.
func boxSlot(s *spawnState, hx, hy float64) physics.ShapeSlot {
	return spawnShape(s, physics.Box(hx, hy).Material(0.3, 0, 1).Filter(0xFFFF, 0xFFFF))
}

const epsilon = 0.001

func pairHas(a, b, x, y cardinal.EntityID) bool {
	return (a == x && b == y) || (a == y && b == x)
}

// initCardinalECS runs the same step as the shard loop before the first Tick: build ECS schedules
// and run Init-hook systems. [ecs.World.Tick] asserts initialized; physics2d_test cannot import
// cardinal/internal/ecs, so we call Init via reflection on Cardinal's embedded *ecs.World.
func initCardinalECS(w *cardinal.World) {
	v := reflect.ValueOf(w).Elem()
	f := v.FieldByName("world")
	if !f.IsValid() {
		panic("cardinal.World: missing embedded ecs world field")
	}
	inner := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	m := inner.MethodByName("Init")
	if !m.IsValid() {
		panic("ecs.World: missing Init method")
	}
	m.Call(nil)
}
