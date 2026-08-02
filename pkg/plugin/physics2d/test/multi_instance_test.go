package physics2d_test

// Multi-instance isolation: with the pure-Go Box2D backend, each Plugin instance owns its own
// physics world, so two Cardinal worlds in one process must simulate fully independently.
// (The old CGO backend had a process-wide singleton C world; this test is the regression gate
// for that limitation staying gone.)

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// spawnBallAndFloor registers an Init system that creates a static floor at y=0 and a dynamic
// ball at (x, 5) with a circle collider, and returns pointers that receive the created ids.
func spawnBallAndFloor(w *cardinal.World, x float64, ballID *cardinal.EntityID) {
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, floor := state.Spawn.Create()
		floor.Tag.Set(harnessTag{Role: "floor"})
		floor.T.Set(physics.Transform2D{Position: physics.Vec2{X: x, Y: 0}})
		floor.V.Set(physics.Velocity2D{})
		floor.PB.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(10, 0.5)...))

		id, ball := state.Spawn.Create()
		ball.Tag.Set(harnessTag{Role: "ball"})
		ball.T.Set(physics.Transform2D{Position: physics.Vec2{X: x, Y: 5}})
		ball.V.Set(physics.Velocity2D{})
		ball.PB.Set(newRigid(physics.BodyTypeDynamic, circleColliderShapes()...))
		*ballID = id
	}, cardinal.WithHook(cardinal.Init))
}

// ballY reads the ball's current Y position through the plugin's raycast-free ECS view: a
// PostUpdate probe system copies it into out each tick.
func trackBallY(w *cardinal.World, out *float64) {
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for _, row := range state.Spawn.Iter() {
			if row.Tag.Get().Role == "ball" {
				*out = row.T.Get().Position.Y
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))
}

// TestMultiInstance_IndependentWorlds proves two plugins in one process simulate independently:
// different gravity per world, interleaved ticking, no cross-talk in body state, engines, or
// queries.
func TestMultiInstance_IndependentWorlds(t *testing.T) {
	t.Parallel()

	// World A: gravity pulls the ball down. World B: zero gravity, ball must not move.
	wA, pA := makeWorld(t, physics.Vec2{X: 0, Y: -10})
	wB, pB := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var ballA, ballB cardinal.EntityID
	var yA, yB float64
	spawnBallAndFloor(wA, 0, &ballA)
	spawnBallAndFloor(wB, 100, &ballB)
	trackBallY(wA, &yA)
	trackBallY(wB, &yB)

	initCardinalECS(wA)
	initCardinalECS(wB)

	// Interleave ticks so any accidental shared backend state would corrupt one of the two.
	for range 60 {
		tickN(t, wA, 1)
		tickN(t, wB, 1)
	}

	require.NotNil(t, pA.Engine(), "world A engine must exist")
	require.NotNil(t, pB.Engine(), "world B engine must exist")
	require.NotSame(t, pA.Engine(), pB.Engine(), "each plugin must own a distinct Box2D world")

	require.Less(t, yA, 4.0, "world A ball must fall under gravity")
	require.InDelta(t, 5.0, yB, 1e-9, "world B ball must not move in zero gravity")

	// Queries are scoped to their own world: world A's ball is at x~0, world B's at x=100.
	rayA := pA.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 100, Y: 5}, End: physics.Vec2{X: 100, Y: 4},
	})
	require.False(t, rayA.Hit, "world A must not see world B's ball")

	rayB := pB.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 100, Y: 6.5}, End: physics.Vec2{X: 100, Y: 3},
	})
	require.True(t, rayB.Hit, "world B must see its own ball")
	require.Equal(t, ballB, rayB.Entity, "world B raycast hits its own ball entity")

	// Resetting one plugin must not touch the other.
	pA.Reset()
	require.Nil(t, pA.Engine(), "world A engine gone after Reset")
	require.NotNil(t, pB.Engine(), "world B engine unaffected by world A Reset")

	tickN(t, wB, 30)
	require.InDelta(t, 5.0, yB, 1e-9, "world B still simulates cleanly after world A Reset")

	// World A rebuilds from ECS on its next tick.
	tickN(t, wA, 1)
	require.NotNil(t, pA.Engine(), "world A engine rebuilt after Reset + tick")
}
