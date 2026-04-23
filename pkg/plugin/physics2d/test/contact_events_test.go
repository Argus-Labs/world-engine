package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ContactBegin + ContactEnd — ball falls onto floor, rests, then is teleported away
// ---------------------------------------------------------------------------

func TestContactEvents_BeginAndEnd(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var ballID, floorID cardinal.EntityID
	contactBeginCount := 0
	contactEndCount := 0

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Floor.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "floor"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 20, Y: 0.5},
			Density:      0,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		floorID = id

		// Ball starts above floor.
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "ball"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 3}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.4,
			Density:      1,
			Restitution:  0,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		ballID = id2
	}, cardinal.WithHook(cardinal.Init))

	// Teleport ball away at tick 60 to break contact.
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
				tr.Position = physics.Vec2{X: 0, Y: 50}
				row.T.Set(tr)
				row.V.Set(physics.Velocity2D{}) // zero velocity
			}
		}
		// Disable gravity on ball so it doesn't fall back.
		for eid, row := range state.Spawn.Iter() {
			if eid == ballID {
				pb := row.PB.Get()
				pb.GravityScale = 0
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	// Collect events.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		ContactBeginRx cardinal.WithSystemEventReceiver[physics.ContactBeginEvent]
		ContactEndRx   cardinal.WithSystemEventReceiver[physics.ContactEndEvent]
	}) {
		for e := range state.ContactBeginRx.Iter() {
			if pairHas(e.EntityA, e.EntityB, ballID, floorID) {
				contactBeginCount++
			}
		}
		for e := range state.ContactEndRx.Iter() {
			if pairHas(e.EntityA, e.EntityB, ballID, floorID) {
				contactEndCount++
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 80)

	require.GreaterOrEqual(t, contactBeginCount, 1, "ContactBegin should fire when ball hits floor")
	require.GreaterOrEqual(t, contactEndCount, 1, "ContactEnd should fire when ball teleported away")
}

// ---------------------------------------------------------------------------
// TriggerBegin + TriggerEnd — ball passes through a sensor
// ---------------------------------------------------------------------------

func TestContactEvents_TriggerBeginAndEnd(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var ballID, sensorID cardinal.EntityID
	triggerBeginCount := 0
	triggerEndCount := 0

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Sensor zone.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "sensor_zone"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 2}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       1.0,
			IsSensor:     true,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		sensorID = id

		// Ball starts above sensor, will fall through it.
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "ball"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 6}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.3,
			Density:      1,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		ballID = id2
	}, cardinal.WithHook(cardinal.Init))

	// Collect trigger events.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		TriggerBeginRx cardinal.WithSystemEventReceiver[physics.TriggerBeginEvent]
		TriggerEndRx   cardinal.WithSystemEventReceiver[physics.TriggerEndEvent]
	}) {
		for e := range state.TriggerBeginRx.Iter() {
			if pairHas(e.EntityA, e.EntityB, ballID, sensorID) {
				triggerBeginCount++
			}
		}
		for e := range state.TriggerEndRx.Iter() {
			if pairHas(e.EntityA, e.EntityB, ballID, sensorID) {
				triggerEndCount++
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 120)

	require.GreaterOrEqual(t, triggerBeginCount, 1, "TriggerBegin should fire when ball enters sensor")
	require.GreaterOrEqual(t, triggerEndCount, 1, "TriggerEnd should fire when ball leaves sensor")
}

// ---------------------------------------------------------------------------
// Sensor toggle mid-sim — solid fixture becomes sensor
// ---------------------------------------------------------------------------

func TestContactEvents_SensorToggleMidSim(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var manualID, targetID cardinal.EntityID
	triggerFired := false

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Manual body that we'll move toward the target.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "mover"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: -5, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeManual, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			Density:      1,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		manualID = id

		// Target: initially solid (not sensor).
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "target"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigidNoGravity(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			Density:      1,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		targetID = id2
	}, cardinal.WithHook(cardinal.Init))

	// Toggle target to sensor at tick 10, then move mover toward it.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() == 10 {
			for eid, row := range state.Spawn.Iter() {
				if eid == targetID {
					pb := row.PB.Get()
					pb.Shapes[0].IsSensor = true
					row.PB.Set(pb)
				}
			}
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == manualID {
				tr := row.T.Get()
				tr.Position.X += 0.3
				row.T.Set(tr)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	// Listen for trigger events.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		TriggerBeginRx cardinal.WithSystemEventReceiver[physics.TriggerBeginEvent]
	}) {
		for e := range state.TriggerBeginRx.Iter() {
			if pairHas(e.EntityA, e.EntityB, manualID, targetID) {
				triggerFired = true
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 60)

	require.True(t, triggerFired,
		"TriggerBegin should fire after solid-to-sensor toggle and overlap")
}

// ---------------------------------------------------------------------------
// Entity destroy mid-contact — ContactEnd/TriggerEnd should not crash
// ---------------------------------------------------------------------------

func TestContactEvents_EntityDestroyDuringOverlap(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var manualID, targetID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Manual body overlapping target from the start.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "mover"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeManual, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       1.0,
			Density:      1,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		manualID = id

		// Target sensor overlapping mover.
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "sensor_target"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigidNoGravity(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       1.0,
			IsSensor:     true,
			Density:      1,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		targetID = id2
	}, cardinal.WithHook(cardinal.Init))

	// Destroy target mid-sim.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 15 {
			return
		}
		state.Spawn.Destroy(targetID)
	}, cardinal.WithHook(cardinal.Update))

	// Just drain events — we only care that it doesn't crash.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		TriggerBeginRx cardinal.WithSystemEventReceiver[physics.TriggerBeginEvent]
		TriggerEndRx   cardinal.WithSystemEventReceiver[physics.TriggerEndEvent]
	}) {
		for range state.TriggerBeginRx.Iter() {
		}
		for range state.TriggerEndRx.Iter() {
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 30)

	// No crash = success. The orphan body cleanup should handle this gracefully.
	_ = manualID
}

// ---------------------------------------------------------------------------
// Multiple simultaneous contacts — ball touching two bodies at once
// ---------------------------------------------------------------------------

func TestContactEvents_MultipleSimultaneous(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var ballID, floorID, wallID cardinal.EntityID
	floorContact := false
	wallContact := false

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Floor.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "floor"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 20, Y: 0.5},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		floorID = id

		// Vertical wall. Left edge at X=1.0 (center 1.5, half-width 0.5).
		// Tall enough (Y extends to 7.5) to catch the ball before it falls past.
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "wall"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 1.5, Y: 2.5}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 5},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		wallID = id2

		// Ball drops with rightward velocity — hits wall, slides down to floor.
		id3, row3 := state.Spawn.Create()
		row3.Tag.Set(harnessTag{Role: "ball"})
		row3.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0.3, Y: 5}})
		row3.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 3, Y: 0}})
		row3.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.4,
			Density:      1,
			Restitution:  0,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		ballID = id3
	}, cardinal.WithHook(cardinal.Init))

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		ContactBeginRx cardinal.WithSystemEventReceiver[physics.ContactBeginEvent]
	}) {
		for e := range state.ContactBeginRx.Iter() {
			if pairHas(e.EntityA, e.EntityB, ballID, floorID) {
				floorContact = true
			}
			if pairHas(e.EntityA, e.EntityB, ballID, wallID) {
				wallContact = true
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 80)

	require.True(t, floorContact, "ball should contact floor")
	require.True(t, wallContact, "ball should contact wall")
}

// ---------------------------------------------------------------------------
// ContactBegin event includes Normal and Point (when manifold exists)
// ---------------------------------------------------------------------------

func TestContactEvents_ManifoldData(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var ballID, floorID cardinal.EntityID
	normalValid := false
	pointValid := false

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "floor"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 20, Y: 0.5},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		floorID = id

		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "ball"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 3}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.4,
			Density:      1,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		ballID = id2
	}, cardinal.WithHook(cardinal.Init))

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		ContactBeginRx cardinal.WithSystemEventReceiver[physics.ContactBeginEvent]
	}) {
		for e := range state.ContactBeginRx.Iter() {
			if pairHas(e.EntityA, e.EntityB, ballID, floorID) {
				normalValid = e.NormalValid
				pointValid = e.PointValid
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 60)

	require.True(t, normalValid, "ContactBegin should include contact normal")
	require.True(t, pointValid, "ContactBegin should include contact point")
}

// ---------------------------------------------------------------------------
// Event names are correct
// ---------------------------------------------------------------------------

func TestContactEvents_EventNames(t *testing.T) {
	t.Parallel()
	require.Equal(t, "physics2d_contact_begin", physics.ContactBeginEvent{}.Name())
	require.Equal(t, "physics2d_contact_end", physics.ContactEndEvent{}.Name())
	require.Equal(t, "physics2d_trigger_begin", physics.TriggerBeginEvent{}.Name())
	require.Equal(t, "physics2d_trigger_end", physics.TriggerEndEvent{}.Name())
}
