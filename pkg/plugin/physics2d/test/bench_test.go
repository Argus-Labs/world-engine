package physics2d_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// benchWorld creates a Cardinal world suitable for benchmarks (no logging).
func benchWorld(b *testing.B, gravity physics.Vec2) *cardinal.World {
	b.Helper()
	b.Setenv("LOG_LEVEL", "disabled")
	debug := true
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		Region:              "local",
		Organization:        "bench",
		Project:             "bench",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1_000_000,
		Debug:               &debug,
	})
	if err != nil {
		b.Fatal(err)
	}
	cardinal.RegisterPlugin(w, physics.NewPlugin(physics.Config{
		Gravity:  gravity,
		TickRate: 60,
	}))
	return w
}

// benchTickN ticks the world n times without test failure checks.
func benchTickN(w *cardinal.World, n int) {
	ctx := context.Background()
	for i := range n {
		w.Tick(ctx, time.Unix(int64(i), 0))
	}
}

// ---------------------------------------------------------------------------
// BenchmarkStep — N dynamic circles falling onto a static floor.
// Measures broadphase + narrowphase + solver throughput per tick.
// ---------------------------------------------------------------------------

func BenchmarkStep(b *testing.B) {
	for _, n := range []int{100, 500, 1000, 5000} {
		b.Run(fmt.Sprintf("Bodies_%d", n), func(b *testing.B) {
			w := benchWorld(b, physics.Vec2{X: 0, Y: -10})
			bodyCount := n

			cardinal.RegisterSystem(w, func(state *struct {
				cardinal.BaseSystemState
				Spawn spawnArchetype
			}) {
				if state.Tick() != 0 {
					return
				}
				// Static floor.
				_, row := state.Spawn.Create()
				row.Tag.Set(harnessTag{Role: "floor"})
				row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: -5}})
				row.V.Set(physics.Velocity2D{})
				row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
					ShapeType:    physics.ShapeTypeBox,
					HalfExtents:  physics.Vec2{X: 200, Y: 1},
					Friction:     0.5,
					CategoryBits: 0xFFFF,
					MaskBits:     0xFFFF,
				}))

				// Spawn N dynamic circles in a grid above the floor.
				cols := int(math.Ceil(math.Sqrt(float64(bodyCount))))
				for i := range bodyCount {
					col := i % cols
					rowIdx := i / cols
					x := float64(col)*2.0 - float64(cols)
					y := float64(rowIdx)*2.0 + 5.0

					_, r := state.Spawn.Create()
					r.Tag.Set(harnessTag{Role: "ball"})
					r.T.Set(physics.Transform2D{Position: physics.Vec2{X: x, Y: y}})
					r.V.Set(physics.Velocity2D{})
					r.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
						ShapeType:    physics.ShapeTypeCircle,
						Radius:       0.5,
						Density:      1,
						Friction:     0.3,
						Restitution:  0.2,
						CategoryBits: 0xFFFF,
						MaskBits:     0xFFFF,
					}))
				}
			}, cardinal.WithHook(cardinal.Init))

			initCardinalECS(w)
			// Warm up: let bodies settle a bit.
			benchTickN(w, 10)

			b.ResetTimer()
			ctx := context.Background()
			for i := range b.N {
				w.Tick(ctx, time.Unix(int64(100+i), 0))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkRaycast — raycast through a scene with N scattered bodies.
// ---------------------------------------------------------------------------

func BenchmarkRaycast(b *testing.B) {
	for _, n := range []int{100, 500, 1000, 5000} {
		b.Run(fmt.Sprintf("Bodies_%d", n), func(b *testing.B) {
			w := benchWorld(b, physics.Vec2{X: 0, Y: 0})
			bodyCount := n

			cardinal.RegisterSystem(w, gridSpawnSystem(bodyCount), cardinal.WithHook(cardinal.Init))

			initCardinalECS(w)
			benchTickN(w, 2) // rebuild + settle

			b.ResetTimer()
			for range b.N {
				physics.Raycast(physics.RaycastRequest{
					Origin: physics.Vec2{X: -500, Y: 0},
					End:    physics.Vec2{X: 500, Y: 0},
					Filter: &physics.Filter{
						CategoryBits:   0xFFFF,
						MaskBits:       0xFFFF,
						IncludeSensors: false,
					},
				})
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkOverlapAABB — AABB overlap query in a dense scene.
// ---------------------------------------------------------------------------

func BenchmarkOverlapAABB(b *testing.B) {
	for _, n := range []int{100, 500, 1000, 5000} {
		b.Run(fmt.Sprintf("Bodies_%d", n), func(b *testing.B) {
			w := benchWorld(b, physics.Vec2{X: 0, Y: 0})
			bodyCount := n

			cardinal.RegisterSystem(w, gridSpawnSystem(bodyCount), cardinal.WithHook(cardinal.Init))

			initCardinalECS(w)
			benchTickN(w, 2)

			b.ResetTimer()
			for range b.N {
				physics.OverlapAABB(physics.AABBOverlapRequest{
					Min: physics.Vec2{X: -10, Y: -10},
					Max: physics.Vec2{X: 10, Y: 10},
					Filter: &physics.Filter{
						CategoryBits:   0xFFFF,
						MaskBits:       0xFFFF,
						IncludeSensors: false,
					},
				})
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkCircleSweep — circle sweep through a dense scene.
// ---------------------------------------------------------------------------

func BenchmarkCircleSweep(b *testing.B) {
	for _, n := range []int{100, 500, 1000, 5000} {
		b.Run(fmt.Sprintf("Bodies_%d", n), func(b *testing.B) {
			w := benchWorld(b, physics.Vec2{X: 0, Y: 0})
			bodyCount := n

			cardinal.RegisterSystem(w, gridSpawnSystem(bodyCount), cardinal.WithHook(cardinal.Init))

			initCardinalECS(w)
			benchTickN(w, 2)

			b.ResetTimer()
			for range b.N {
				physics.CircleSweep(physics.CircleSweepRequest{
					Start:  physics.Vec2{X: -500, Y: 0},
					End:    physics.Vec2{X: 500, Y: 0},
					Radius: 2.0,
					Filter: &physics.Filter{
						CategoryBits:   0xFFFF,
						MaskBits:       0xFFFF,
						IncludeSensors: false,
					},
				})
			}
		})
	}
}

// gridSpawnSystem returns a system that spawns count static circles in a grid on tick 0.
func gridSpawnSystem(count int) func(state *struct {
	cardinal.BaseSystemState
	Spawn spawnArchetype
}) {
	return func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		cols := int(math.Ceil(math.Sqrt(float64(count))))
		spacing := 3.0
		for i := range count {
			col := i % cols
			rowIdx := i / cols
			x := float64(col)*spacing - float64(cols)*spacing/2
			y := float64(rowIdx)*spacing - float64(cols)*spacing/2

			_, r := state.Spawn.Create()
			r.Tag.Set(harnessTag{Role: "grid"})
			r.T.Set(physics.Transform2D{Position: physics.Vec2{X: x, Y: y}})
			r.V.Set(physics.Velocity2D{})
			r.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
				ShapeType:    physics.ShapeTypeCircle,
				Radius:       1.0,
				Friction:     0.3,
				CategoryBits: 0xFFFF,
				MaskBits:     0xFFFF,
			}))
		}
	}
}
