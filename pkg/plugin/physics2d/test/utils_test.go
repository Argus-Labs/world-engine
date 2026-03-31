package physics2d_test

import (
	"context"
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

// makeWorld creates a Cardinal world at 60 Hz with the physics plugin installed.
// World gravity is set from gravity; plugin tick rate matches the world tick rate.
func makeWorld(t *testing.T, gravity physics.Vec2) *cardinal.World {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")
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
	cardinal.RegisterPlugin(w, physics.NewPlugin(physics.Config{
		Gravity:  gravity,
		TickRate: 60,
	}))
	return w
}

// newRigid returns a Rigidbody2D with Active/Awake/SleepingAllowed true and GravityScale 1.
func newRigid(bodyType physics.BodyType) physics.Rigidbody2D {
	return phycomp.NewRigidbody2D(bodyType)
}

// newRigidNoGravity is like newRigid but GravityScale 0 (e.g. zero-gravity scene bodies).
func newRigidNoGravity(bodyType physics.BodyType) physics.Rigidbody2D {
	r := phycomp.NewRigidbody2D(bodyType)
	r.GravityScale = 0
	return r
}

func tickN(t *testing.T, w *cardinal.World, n int) {
	t.Helper()
	ctx := context.Background()
	for i := range n {
		w.Tick(ctx, time.Unix(int64(i), 0))
		if t.Failed() {
			t.Fatalf("failed at tick %d", i)
		}
	}
}

func circleCollider() physics.Collider2D {
	return physics.Collider2D{Shapes: []physics.ColliderShape{{
		ShapeType:    physics.ShapeTypeCircle,
		Radius:       0.5,
		Density:      1,
		Friction:     0.3,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}}}
}

func boxCollider(hx, hy float64) physics.Collider2D {
	return physics.Collider2D{Shapes: []physics.ColliderShape{{
		ShapeType:    physics.ShapeTypeBox,
		HalfExtents:  physics.Vec2{X: hx, Y: hy},
		Density:      1,
		Friction:     0.3,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}}}
}

const epsilon = 0.001

func approxVec2(t *testing.T, got, want physics.Vec2, msg string) {
	t.Helper()
	require.InDelta(t, want.X, got.X, epsilon, "%s X", msg)
	require.InDelta(t, want.Y, got.Y, epsilon, "%s Y", msg)
}

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
