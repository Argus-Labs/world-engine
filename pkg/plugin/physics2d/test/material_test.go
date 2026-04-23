package physics2d_test

import (
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Restitution — high restitution ball bounces higher than low restitution ball
// ---------------------------------------------------------------------------

func TestMaterial_Restitution(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var bouncyID, deadID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Floor with zero restitution. Box2D uses max(a,b) restitution mixing, so setting
		// the floor to 0 lets the ball's own restitution determine bounce behavior.
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "floor"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 30, Y: 0.5},
			Restitution:  0.0,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))

		// Bouncy ball (restitution=1.0).
		id, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "bouncy"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: -5, Y: 5}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.3,
			Density:      1,
			Restitution:  1.0,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		bouncyID = id

		// Dead ball (restitution=0.0).
		id2, row3 := state.Spawn.Create()
		row3.Tag.Set(harnessTag{Role: "dead"})
		row3.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 5}})
		row3.V.Set(physics.Velocity2D{})
		row3.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.3,
			Density:      1,
			Restitution:  0.0,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		deadID = id2
	}, cardinal.WithHook(cardinal.Init))

	// Sample Y velocity after enough ticks for first bounce.
	bouncyMaxY := 0.0
	deadMaxY := 0.0
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		// After initial landing (~60 ticks), track max Y.
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			y := row.T.Get().Position.Y
			if eid == bouncyID && y > bouncyMaxY {
				bouncyMaxY = y
			}
			if eid == deadID && y > deadMaxY {
				deadMaxY = y
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 180)

	require.Greater(t, bouncyMaxY, deadMaxY+0.5,
		"bouncy ball should reach higher than dead ball after bounce")
}

// ---------------------------------------------------------------------------
// Restitution change mid-sim — changing restitution affects bounce
// ---------------------------------------------------------------------------

func TestMaterial_RestitutionChangeMidSim(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var ballID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Floor.
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "floor"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 20, Y: 0.5},
			Restitution:  1.0,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))

		// Ball starts with zero restitution.
		id, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "ball"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 5}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.3,
			Density:      1,
			Restitution:  0.0,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		ballID = id
	}, cardinal.WithHook(cardinal.Init))

	// After landing (tick 60), teleport ball up and change restitution to 1.0.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == ballID {
				tr := row.T.Get()
				tr.Position = physics.Vec2{X: 0, Y: 5}
				row.T.Set(tr)
				row.V.Set(physics.Velocity2D{})
				pb := row.PB.Get()
				pb.Shapes[0].Restitution = 1.0
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	// Track Y velocity after second landing to verify bounce.
	maxYAfterSecondDrop := 0.0
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 120 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == ballID {
				y := row.T.Get().Position.Y
				if y > maxYAfterSecondDrop {
					maxYAfterSecondDrop = y
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 180)

	// With restitution=1.0, ball should bounce back up to near starting height.
	require.Greater(t, maxYAfterSecondDrop, 2.0,
		"ball with restitution=1.0 should bounce back up significantly")
}

// ---------------------------------------------------------------------------
// Gravity scale change mid-sim — dynamic body responds to gravity change
// ---------------------------------------------------------------------------

func TestMaterial_GravityScaleChangeMidSim(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Start with no gravity.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "grav_toggle"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 20}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	// Enable gravity at tick 30.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				pb := row.PB.Get()
				pb.GravityScale = 1.0
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	posAtChange := 0.0
	var finalPos float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				if state.Tick() == 30 {
					posAtChange = row.T.Get().Position.Y
				}
				if state.Tick() == 90 {
					finalPos = row.T.Get().Position.Y
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 91)

	require.InDelta(t, 20.0, posAtChange, 0.1, "body should not move with gravity_scale=0")
	require.Less(t, finalPos, posAtChange-1.0, "body should fall after enabling gravity")
}

// ---------------------------------------------------------------------------
// Kinematic rotation writeback — angular velocity integrated
// ---------------------------------------------------------------------------

func TestMaterial_KinematicRotation(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "kin_spinner"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{Angular: 3.0})
		row.PB.Set(newRigid(physics.BodyTypeKinematic, circleColliderShapes()...))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	var finalRot float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				finalRot = row.T.Get().Rotation
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	// 60 ticks at 60Hz = 1s at 3 rad/s → ~3 radians.
	require.Greater(t, math.Abs(finalRot), 2.0,
		"kinematic body should rotate via angular velocity writeback")
}
