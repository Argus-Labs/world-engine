// Package physics2d_test holds Cardinal-driven integration tests for the physics2d plugin.
// Tests exercise real tick ordering across PreUpdate / Update / PostUpdate; the plugin runs the
// physics pipeline (reconcile → step → writeback) on PreUpdate unless tests override registration.
package physics2d_test

import (
	"context"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

type harnessTag struct {
	Role string `json:"role"`
}

func (harnessTag) Name() string { return "physics2d_e2e_harness_tag" }

type spawnArchetype = cardinal.Exact[struct {
	Tag cardinal.Ref[harnessTag]
	T   cardinal.Ref[physics.Transform2D]
	V   cardinal.Ref[physics.Velocity2D]
	PB  cardinal.Ref[physics.PhysicsBody2D]
}]

var harness struct {
	Floor, Ball, Sensor, FilterWall, Triangle cardinal.EntityID
	SecondBall, CompoundBody                  cardinal.EntityID
	NewBox                                    cardinal.EntityID
	KinematicMover, ManualPlayer, Spinner     cardinal.EntityID
}

const (
	tickTriggerDeadline   = 30
	tickContactDeadline   = 120
	tickDestroyTriangle   = 150
	tickMoveWall          = 170
	tickCreateNewBox      = 190
	tickCrash1            = 200
	tickCrash1Verify      = 204
	tickPostCrash1Trigger = 250
	tickPostCrash1Contact = 340
	tickCrash2            = 350
	tickCrash2Verify      = 355
)

var (
	crashPhase       uint32
	contactBeginCnt  uint32
	triggerBeginCnt  uint32
	triangleGone     uint32
	wallMoved        uint32
	newBoxCreated    uint32
	crash1EndContact uint32
	crash1EndTrigger uint32
	crash2EndContact uint32
	crash2EndTrigger uint32
	crash2NewBegin   uint32
)

// Writeback tracking — populated each tick by verifySystem, checked at milestone ticks.
var (
	ballPosYHistory      []float64
	ballVelYHistory      []float64
	kinematicPosXHistory []float64
	spinnerRotHistory    []float64
	postCrash1BallPosY   []float64
	manualSpawnPos       physics.Vec2
)

var testRequire *require.Assertions

// sceneInitSystem runs on [cardinal.Init]. It spawns the harness scene so the plugin’s Init
// FullRebuildFromECS sees all bodies: floor, falling ball, sensor, layered wall, triangle, chain,
// zero-gravity second ball, and a static compound collider (box + offset sensor circle).
func sceneInitSystem(state *struct {
	cardinal.BaseSystemState
	Spawn spawnArchetype
}) {
	mustCreate := func(
		role string,
		t physics.Transform2D,
		pb physics.PhysicsBody2D,
	) cardinal.EntityID {
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: role})
		row.T.Set(t)
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(pb)
		return id
	}
	mustCreateWithVel := func(
		role string,
		t physics.Transform2D,
		v physics.Velocity2D,
		pb physics.PhysicsBody2D,
	) cardinal.EntityID {
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: role})
		row.T.Set(t)
		row.V.Set(v)
		row.PB.Set(pb)
		return id
	}

	// Static floor (wide box), top at y=0.
	harness.Floor = mustCreate("floor",
		physics.Transform2D{Position: physics.Vec2{X: 0, Y: -0.25}},
		newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 20, Y: 0.25},
			Friction:     0.6,
			Density:      0,
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}),
	)

	// Dynamic ball; starts above sensor path so TriggerBegin fires after a few steps (not at t=0 overlap).
	harness.Ball = mustCreate("ball",
		physics.Transform2D{Position: physics.Vec2{X: 0, Y: 5.2}},
		newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.4,
			Friction:     0.3,
			Restitution:  0.05,
			Density:      1,
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}),
	)

	// Large sensor on ball’s fall line (trigger overlap tests).
	harness.Sensor = mustCreate("sensor",
		physics.Transform2D{Position: physics.Vec2{X: 0, Y: 2}},
		newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       2.5,
			IsSensor:     true,
			Density:      0,
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}),
	)

	// Solid wall on category 0x0002 for raycast / sweep filter tests.
	harness.FilterWall = mustCreate("filter_wall",
		physics.Transform2D{Position: physics.Vec2{X: 15, Y: 0.5}},
		newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.2, Y: 2},
			Friction:     0.5,
			Density:      0,
			CategoryBits: 0x0002,
			MaskBits:     0xFFFF,
		}),
	)

	// Convex polygon; destroyed mid-scenario to test orphan body cleanup.
	harness.Triangle = mustCreate("triangle",
		physics.Transform2D{Position: physics.Vec2{X: -8, Y: 1}},
		newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType: physics.ShapeTypeConvexPolygon,
			Vertices: []physics.Vec2{
				{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 1, Y: 1.5},
			},
			Friction:     0.5,
			Density:      0,
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}),
	)

	// Static chain segment (extra shape-type coverage); not referenced by assertions.
	_ = mustCreate("chain_ramp",
		physics.Transform2D{Position: physics.Vec2{X: -15, Y: 0}},
		newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType: physics.ShapeTypeStaticChain,
			ChainPoints: []physics.Vec2{
				{X: 0, Y: 0}, {X: 1.5, Y: 0.2}, {X: 3, Y: 0.4}, {X: 4, Y: 0.5},
			},
			Friction:     0.4,
			Density:      0,
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}),
	)

	// Extra dynamic body, no gravity (scene filler; main ball drives contact tests).
	harness.SecondBall = mustCreate("second_ball",
		physics.Transform2D{Position: physics.Vec2{X: 5, Y: 20}},
		newRigidNoGravity(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.4,
			Friction:     0.3,
			Restitution:  0.05,
			Density:      1,
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}),
	)

	// Two fixtures: solid box + offset sensor circle (compound + query IncludeSensors tests).
	harness.CompoundBody = mustCreate("compound_body",
		physics.Transform2D{Position: physics.Vec2{X: -12, Y: 1}},
		newRigid(physics.BodyTypeStatic,
			physics.ColliderShape{
				ShapeType:    physics.ShapeTypeBox,
				HalfExtents:  physics.Vec2{X: 0.5, Y: 0.5},
				Friction:     0.5,
				Density:      0,
				CategoryBits: 0x0001,
				MaskBits:     0xFFFF,
			},
			physics.ColliderShape{
				ShapeType:    physics.ShapeTypeCircle,
				Radius:       0.3,
				IsSensor:     true,
				LocalOffset:  physics.Vec2{X: 0, Y: 1.5},
				Density:      0,
				CategoryBits: 0x0001,
				MaskBits:     0xFFFF,
			},
		),
	)

	// --- Writeback-coverage entities (isolated on category 0x0004, no collisions with main scene) ---

	// Kinematic mover: constant rightward velocity; Box2D integrates it, writeback updates ECS X.
	harness.KinematicMover = mustCreateWithVel("kinematic_mover",
		physics.Transform2D{Position: physics.Vec2{X: 20, Y: 5}},
		physics.Velocity2D{Linear: physics.Vec2{X: 3, Y: 0}},
		newRigidNoGravity(physics.BodyTypeKinematic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.3,
			Density:      0,
			CategoryBits: 0x0004,
			MaskBits:     0x0004,
		}),
	)

	// Manual player: ECS-driven position; writeback must be skipped (no gravity fall, no drift).
	manualSpawnPos = physics.Vec2{X: 30, Y: 5}
	harness.ManualPlayer = mustCreate("manual_player",
		physics.Transform2D{Position: manualSpawnPos},
		newRigid(physics.BodyTypeManual, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			Density:      1,
			CategoryBits: 0x0004,
			MaskBits:     0x0004,
		}),
	)

	// Spinner: dynamic body, zero gravity, angular velocity 2 rad/s; tests rotation writeback.
	spinnerBody := newRigidNoGravity(physics.BodyTypeDynamic, physics.ColliderShape{
		ShapeType:    physics.ShapeTypeCircle,
		Radius:       0.3,
		Density:      1,
		CategoryBits: 0x0004,
		MaskBits:     0x0004,
	})
	spinnerBody.SleepingAllowed = false
	harness.Spinner = mustCreateWithVel("spinner",
		physics.Transform2D{Position: physics.Vec2{X: 25, Y: 10}},
		physics.Velocity2D{Angular: 2.0},
		spinnerBody,
	)
}

// manualMoveSystem runs on [cardinal.PreUpdate] and moves the ManualPlayer 0.05 units right
// each tick, simulating gameplay code that owns the entity's position.
func manualMoveSystem(state *struct {
	cardinal.BaseSystemState
	Spawn spawnArchetype
}) {
	if harness.ManualPlayer == 0 {
		return
	}
	for eid, row := range state.Spawn.Iter() {
		if eid == harness.ManualPlayer {
			tr := row.T.Get()
			tr.Position.X += 0.05
			row.T.Set(tr)
			break
		}
	}
}

// verifySystem runs on [cardinal.PostUpdate] after the physics step. It drains contact/trigger
// receivers, enforces tick-based deadlines (pre-crash, post-crash1, post-crash2), runs query checks,
// applies scripted ECS edits (destroy triangle, move wall, create NewBox, teleport ball, ResetRuntime),
// verifies writeback round-trip (ECS reflects Box2D simulation), and asserts synthetic recovery
// events match expectations.
//
//nolint:cyclop,gocyclo // linear scenario script (same phases as townhall harness)
func verifySystem(state *struct {
	cardinal.BaseSystemState
	Spawn          spawnArchetype
	ContactBeginRx cardinal.WithSystemEventReceiver[physics.ContactBeginEvent]
	ContactEndRx   cardinal.WithSystemEventReceiver[physics.ContactEndEvent]
	TriggerBeginRx cardinal.WithSystemEventReceiver[physics.TriggerBeginEvent]
	TriggerEndRx   cardinal.WithSystemEventReceiver[physics.TriggerEndEvent]
}) {
	req := testRequire
	tick := state.Tick()
	// Cardinal reports tick 0 on first frame; physics assertions start from tick 1.
	if tick == 0 {
		return
	}

	// After ResetRuntime, World is gone until next PreUpdate FullRebuild — only allowed in crash windows.
	if physics.WorldID() == 0 {
		phase := atomic.LoadUint32(&crashPhase)
		if phase == 0 {
			req.FailNow("WorldID() == 0 before any crash")
		}
		if phase >= 1 && tick > tickCrash1+1 && tick <= tickCrash2 {
			req.FailNow("WorldID() == 0 after crash1 — FullRebuildFromECS failed")
		}
		if phase >= 2 && tick > tickCrash2+1 {
			req.FailNow("WorldID() == 0 after crash2 — FullRebuildFromECS failed")
		}
		return
	}

	phase := atomic.LoadUint32(&crashPhase)

	// Collect physics2d system events emitted during this tick’s step (receivers clear each tick).
	for e := range state.ContactBeginRx.Iter() {
		if pairHas(e.EntityA, e.EntityB, harness.Ball, harness.Floor) {
			atomic.AddUint32(&contactBeginCnt, 1)
		}
		if harness.NewBox != 0 && pairHas(e.EntityA, e.EntityB, harness.Ball, harness.NewBox) && phase == 2 {
			atomic.StoreUint32(&crash2NewBegin, 1)
		}
	}
	for e := range state.ContactEndRx.Iter() {
		if pairHas(e.EntityA, e.EntityB, harness.Ball, harness.Floor) {
			switch phase {
			case 1:
				atomic.StoreUint32(&crash1EndContact, 1)
			case 2:
				atomic.StoreUint32(&crash2EndContact, 1)
			}
		}
	}
	for e := range state.TriggerBeginRx.Iter() {
		if pairHas(e.EntityA, e.EntityB, harness.Ball, harness.Sensor) {
			atomic.AddUint32(&triggerBeginCnt, 1)
		}
	}
	for e := range state.TriggerEndRx.Iter() {
		if pairHas(e.EntityA, e.EntityB, harness.Ball, harness.Sensor) {
			switch phase {
			case 1:
				atomic.StoreUint32(&crash1EndTrigger, 1)
			case 2:
				atomic.StoreUint32(&crash2EndTrigger, 1)
			}
		}
	}

	// --- Writeback verification: read ECS state written back by Box2D each tick ---
	for eid, row := range state.Spawn.Iter() {
		switch eid {
		case harness.Ball:
			t := row.T.Get()
			v := row.V.Get()
			if phase == 0 && tick < tickCrash1 {
				ballPosYHistory = append(ballPosYHistory, t.Position.Y)
				ballVelYHistory = append(ballVelYHistory, v.Linear.Y)
			}
			if phase == 1 && tick > tickCrash1+2 && tick < tickCrash2 {
				postCrash1BallPosY = append(postCrash1BallPosY, t.Position.Y)
			}
		case harness.SecondBall:
			if phase == 0 {
				pos := row.T.Get().Position
				req.InDelta(20, pos.Y, 0.5,
					"second ball (zero gravity) ECS Y must stay near spawn at tick %d", tick)
			}
		case harness.KinematicMover:
			if phase == 0 && tick < tickCrash1 {
				kinematicPosXHistory = append(kinematicPosXHistory, row.T.Get().Position.X)
			}
		case harness.ManualPlayer:
			pos := row.T.Get().Position
			if phase == 0 {
				req.InDelta(manualSpawnPos.Y, pos.Y, epsilon,
					"manual body Y unchanged at tick %d (writeback must not clobber)", tick)
			}
			if tick == tickContactDeadline && phase == 0 {
				req.Greater(pos.X, manualSpawnPos.X+2.0,
					"manual body X advanced by gameplay (writeback did not reset it)")
			}
		case harness.Spinner:
			if phase == 0 && tick < tickCrash1 {
				spinnerRotHistory = append(spinnerRotHistory, row.T.Get().Rotation)
			}
		}
	}

	// Before crash 1: normal fall must produce trigger then solid contact (live Box2D callbacks).
	if tick >= tickTriggerDeadline && tick < tickCrash1 {
		req.NotZero(atomic.LoadUint32(&triggerBeginCnt), "TriggerBegin ball-sensor by deadline")
	}
	if tick >= tickContactDeadline && tick < tickCrash1 {
		req.NotZero(atomic.LoadUint32(&contactBeginCnt), "ContactBegin ball-floor by deadline")
	}

	// Writeback milestone: at contact deadline the ball has landed and all tracked bodies are stable.
	if tick == tickContactDeadline && phase == 0 {
		// Ball fell from Y=5.2 to near the floor; positions must be non-increasing (no snap-back).
		n := len(ballPosYHistory)
		req.GreaterOrEqual(n, 50, "enough ball position samples for smooth motion check")
		// Threshold 0.5 catches snap-back (reconciler fighting writeback → multi-meter jumps)
		// while allowing tiny physics bounces from restitution.
		for i := 1; i < n; i++ {
			req.LessOrEqual(ballPosYHistory[i], ballPosYHistory[i-1]+0.5,
				"ball Y jumped up at sample %d (snap-back = writeback/shadow desync)", i)
		}
		// Ball resting near floor (floor top Y=0, ball radius 0.4).
		req.Less(ballPosYHistory[n-1], 2.0, "ball ECS Y near floor after landing")
		// Velocity near zero once resting.
		req.InDelta(0, ballVelYHistory[len(ballVelYHistory)-1], 2.0,
			"ball ECS velocity Y near zero after landing")

		// Kinematic mover: velocity-driven displacement via writeback.
		nk := len(kinematicPosXHistory)
		req.GreaterOrEqual(nk, 50, "enough kinematic position samples")
		// At 3 m/s, 120 ticks at 60 Hz = 2s → start 20 + 6 = 26.
		req.Greater(kinematicPosXHistory[nk-1], 24.0,
			"kinematic mover ECS X advanced by velocity integration writeback")
		for i := 1; i < nk; i++ {
			req.GreaterOrEqual(kinematicPosXHistory[i], kinematicPosXHistory[i-1]-epsilon,
				"kinematic X non-decreasing at sample %d (shadow/writeback consistency)", i)
		}

		// Spinner: angular velocity writeback → ECS rotation non-zero.
		ns := len(spinnerRotHistory)
		req.GreaterOrEqual(ns, 50, "enough spinner rotation samples")
		lastRot := spinnerRotHistory[ns-1]
		req.Greater(math.Abs(lastRot), 1.0,
			"spinner ECS rotation should reflect angular velocity writeback, got %f", lastRot)
	}

	// Raycast, AABB, sweep, compound collider — every tick while world exists.
	runQueryChecks(req)

	// Reconcile: destroy entity → Box2D body removed (triangle no longer in overlap query).
	if tick == tickDestroyTriangle {
		if atomic.CompareAndSwapUint32(&triangleGone, 0, 1) {
			req.True(state.Spawn.Destroy(harness.Triangle), "Destroy(triangle)")
		}
	}
	if tick >= tickDestroyTriangle+2 {
		assertTriangleGone(req)
	}

	// Reconcile: ECS transform change only → SetTransform in Box2D (short ray proves new X).
	if tick == tickMoveWall {
		if atomic.CompareAndSwapUint32(&wallMoved, 0, 1) {
			for eid, row := range state.Spawn.Iter() {
				if eid == harness.FilterWall {
					tr := row.T.Get()
					tr.Position.X = 10
					row.T.Set(tr)
					break
				}
			}
		}
	}
	if tick >= tickMoveWall+2 && atomic.LoadUint32(&wallMoved) != 0 {
		assertWallMoved(req)
	}

	// Reconcile: new physics archetype mid-sim → create body on next PreUpdate.
	if tick == tickCreateNewBox {
		if atomic.CompareAndSwapUint32(&newBoxCreated, 0, 1) {
			id, row := state.Spawn.Create()
			row.Tag.Set(harnessTag{Role: "new_box"})
			row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 1}})
			row.V.Set(physics.Velocity2D{})
			row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
				ShapeType:    physics.ShapeTypeBox,
				HalfExtents:  physics.Vec2{X: 0.5, Y: 0.5},
				Friction:     0.5,
				Density:      0,
				CategoryBits: 0x0001,
				MaskBits:     0xFFFF,
			}))
			harness.NewBox = id
		}
	}
	if tick >= tickCreateNewBox+2 && harness.NewBox != 0 {
		assertNewBoxExists(req)
	}

	// Crash 1: move ball back to spawn height (writeback put it on the floor), then drop Box2D.
	// Rebuild from ECS spawn height means no floor/sensor overlap while active_contacts still
	// lists old pairs → suppressed step diff emits synthetic Ends.
	if tick == tickCrash1 {
		atomic.StoreUint32(&crashPhase, 1)
		for eid, row := range state.Spawn.Iter() {
			if eid == harness.Ball {
				tr := row.T.Get()
				tr.Position = physics.Vec2{X: 0, Y: 5.2}
				row.T.Set(tr)
			}
		}
		physics.ResetRuntime()
		return
	}

	// Post–crash 1: confirm synthetic ContactEnd + TriggerEnd; then ball falls again → second Begins.
	if tick >= tickCrash1Verify && phase == 1 && tick < tickCrash2 {
		req.NotZero(atomic.LoadUint32(&crash1EndContact), "synthetic ContactEnd ball-floor after crash1")
		req.NotZero(atomic.LoadUint32(&crash1EndTrigger), "synthetic TriggerEnd ball-sensor after crash1")
	}
	if tick >= tickPostCrash1Trigger && phase >= 1 && tick < tickCrash2 {
		req.GreaterOrEqual(atomic.LoadUint32(&triggerBeginCnt), uint32(2), "TriggerBegin again after crash1")
	}
	if tick >= tickPostCrash1Contact && phase >= 1 && tick < tickCrash2 {
		req.GreaterOrEqual(atomic.LoadUint32(&contactBeginCnt), uint32(2), "ContactBegin again after crash1")
	}
	// Post-crash1 writeback: ball fell from 5.2 again, ECS position reflects the second landing.
	if tick == tickPostCrash1Contact && phase >= 1 {
		np := len(postCrash1BallPosY)
		req.GreaterOrEqual(np, 50, "enough post-crash1 ball position samples")
		req.Less(postCrash1BallPosY[np-1], 2.0,
			"ball ECS Y near floor after crash1 (writeback round-trip through rebuild)")
	}

	// Crash 2: move ball in ECS onto NewBox, then ResetRuntime. Persisted pairs are floor/sensor;
	// live has ball–NewBox only → diff emits Ends for stale pairs + Begin for new overlap.
	if tick == tickCrash2 {
		atomic.StoreUint32(&crashPhase, 2)
		for eid, row := range state.Spawn.Iter() {
			if eid == harness.Ball {
				tr := row.T.Get()
				tr.Position = physics.Vec2{X: 5, Y: 1.3}
				row.T.Set(tr)
			}
		}
		physics.ResetRuntime()
		return
	}

	// Post–crash 2: all three diff outcomes (two Ends + one Begin) observed this tick or shortly after.
	if tick >= tickCrash2Verify && phase == 2 {
		req.NotZero(atomic.LoadUint32(&crash2EndContact), "synthetic ContactEnd ball-floor after crash2")
		req.NotZero(atomic.LoadUint32(&crash2EndTrigger), "synthetic TriggerEnd ball-sensor after crash2")
		req.NotZero(atomic.LoadUint32(&crash2NewBegin), "synthetic ContactBegin ball-NewBox after crash2")
	}
}

// runQueryChecks exercises [physics.Raycast], [physics.OverlapAABB] (with and without sensor filter),
// and [physics.CircleSweep] against the current Box2D world.
func runQueryChecks(req *require.Assertions) {
	// Raycast with nil filter: hits FilterWall on layer 0x0002.
	rayDef := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 8, Y: 0.25},
		End:    physics.Vec2{X: 25, Y: 0.25},
	})
	req.True(rayDef.Hit && rayDef.Entity == harness.FilterWall,
		"raycast default: want FilterWall %d hit=%v entity=%d", harness.FilterWall, rayDef.Hit, rayDef.Entity)
	req.Zero(rayDef.ShapeIndex, "raycast shape_index")

	// Raycast with mask that does not include 0x0002 → must not hit FilterWall.
	fl := physics.Filter{CategoryBits: 0x0001, MaskBits: 0x0001, IncludeSensors: false}
	rayFilt := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 8, Y: 0.25},
		End:    physics.Vec2{X: 25, Y: 0.25},
		Filter: &fl,
	})
	req.False(rayFilt.Hit, "filtered raycast should miss wall on 0x0002")

	// AABB narrow-phase over triangle region (until entity destroyed).
	if atomic.LoadUint32(&triangleGone) == 0 {
		ov := physics.OverlapAABB(physics.AABBOverlapRequest{
			Min: physics.Vec2{X: -9, Y: 0.5},
			Max: physics.Vec2{X: -6, Y: 2.5},
		})
		found := false
		for _, h := range ov.Hits {
			if h.Entity == harness.Triangle && h.ShapeIndex == 0 {
				found = true
				break
			}
		}
		req.True(found, "OverlapAABB triangle region")
	}

	// Circle cast along x through FilterWall strip.
	sweep := physics.CircleSweep(physics.CircleSweepRequest{
		Start:  physics.Vec2{X: 25, Y: 0.25},
		End:    physics.Vec2{X: -25, Y: 0.25},
		Radius: 0.2,
	})
	req.True(sweep.Hit && sweep.Entity == harness.FilterWall,
		"CircleSweep FilterWall: hit=%v entity=%d", sweep.Hit, sweep.Entity)

	// Default query filter skips sensors → compound body should report only shape 0 (box).
	ovComp := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -13, Y: 0},
		Max: physics.Vec2{X: -11, Y: 3},
	})
	foundBox := false
	for _, h := range ovComp.Hits {
		if h.Entity == harness.CompoundBody {
			req.NotEqual(1, h.ShapeIndex, "unfiltered AABB must not report sensor shape 1")
			if h.ShapeIndex == 0 {
				foundBox = true
			}
		}
	}
	req.True(foundBox, "compound body box shape in AABB")

	// Explicit IncludeSensors → both box (0) and sensor circle (1) should appear.
	incl := physics.Filter{CategoryBits: 0xFFFF, MaskBits: 0xFFFF, IncludeSensors: true}
	ovS := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min:    physics.Vec2{X: -13, Y: 0},
		Max:    physics.Vec2{X: -11, Y: 3},
		Filter: &incl,
	})
	var s0, s1 bool
	for _, h := range ovS.Hits {
		if h.Entity != harness.CompoundBody {
			continue
		}
		if h.ShapeIndex == 0 {
			s0 = true
		}
		if h.ShapeIndex == 1 {
			s1 = true
		}
	}
	req.True(s0 && s1, "IncludeSensors AABB both compound shapes")
}

// assertTriangleGone checks that destroying the triangle entity removed its fixtures from overlap queries.
func assertTriangleGone(req *require.Assertions) {
	ov := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -9, Y: 0.5},
		Max: physics.Vec2{X: -6, Y: 2.5},
	})
	for _, h := range ov.Hits {
		req.NotEqual(harness.Triangle, h.Entity, "triangle should not appear in OverlapAABB after destroy")
	}
}

// assertWallMoved checks transform reconcile: FilterWall moved in ECS should be hit by a short ray
// that would miss at the pre-move wall X.
func assertWallMoved(req *require.Assertions) {
	shortRay := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 8, Y: 0.25},
		End:    physics.Vec2{X: 12, Y: 0.25},
	})
	req.True(shortRay.Hit && shortRay.Entity == harness.FilterWall,
		"short ray after wall move: hit=%v entity=%d", shortRay.Hit, shortRay.Entity)
}

// assertNewBoxExists checks mid-tick entity creation: NewBox should appear in an AABB overlap query.
func assertNewBoxExists(req *require.Assertions) {
	ov := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: 4, Y: 0},
		Max: physics.Vec2{X: 6, Y: 2},
	})
	found := false
	for _, h := range ov.Hits {
		if h.Entity == harness.NewBox {
			found = true
			break
		}
	}
	req.True(found, "NewBox in AABB")
}

// resetHarnessGlobals clears package-level IDs and atomic flags so a single test run starts clean.
func resetHarnessGlobals() {
	crashPhase = 0
	contactBeginCnt = 0
	triggerBeginCnt = 0
	triangleGone = 0
	wallMoved = 0
	newBoxCreated = 0
	crash1EndContact = 0
	crash1EndTrigger = 0
	crash2EndContact = 0
	crash2EndTrigger = 0
	crash2NewBegin = 0
	ballPosYHistory = nil
	ballVelYHistory = nil
	kinematicPosXHistory = nil
	spinnerRotHistory = nil
	postCrash1BallPosY = nil
	manualSpawnPos = physics.Vec2{}
	harness = struct {
		Floor, Ball, Sensor, FilterWall, Triangle cardinal.EntityID
		SecondBall, CompoundBody                  cardinal.EntityID
		NewBox                                    cardinal.EntityID
		KinematicMover, ManualPlayer, Spinner     cardinal.EntityID
	}{}
}

// TestPhysics2D_CardinalIntegration drives physics2d through a real [cardinal.World] for ~360 ticks.
//
// It verifies:
//   - Cardinal ↔ plugin wiring: ECS Init (schedules + Init hooks) before first Tick, plugin Init
//     before first step, and
//     [physics.PhysicsWorld] present except immediately after [physics.ResetRuntime] (rebuilt next PreUpdate).
//   - Contact/trigger system events: TriggerBegin and ContactBegin (ball vs sensor / floor) within
//     deadlines; after crash 1, synthetic TriggerEnd + ContactEnd (persisted active_contacts vs
//     rebuilt Box2D with ECS spawn transform); ball falls again → second begin pair; crash 2 moves
//     ball in ECS onto NewBox + ResetRuntime → synthetic ends for old pairs + ContactBegin for ball–NewBox.
//   - Query API each tick: default raycast vs FilterWall; filtered raycast misses wall on category mask;
//     OverlapAABB for triangle until destroyed; CircleSweep to FilterWall; compound body AABB without
//     filter (sensor shape excluded) and with IncludeSensors (both shapes).
//   - Incremental reconcile: destroy triangle (orphan body removed from queries); move FilterWall in ECS
//     (transform reconcile, short raycast); spawn NewBox mid-sim (creation reconcile, AABB).
//   - Writeback round-trip: dynamic ball ECS Transform2D/Velocity2D reflect Box2D simulation (smooth
//     fall with monotonically decreasing Y, floor landing, near-zero resting velocity); kinematic mover
//     velocity-driven displacement written to ECS; spinner angular velocity → ECS rotation; manual body
//     position unchanged by writeback (ECS-driven via gameplay PreUpdate system); shadow consistency
//     (no snap-back oscillation). Post-crash1 writeback: ball falls from teleport height again.
//
// It does not: restore from JetStream/S3 snapshots or run FromProto (Nop snapshot storage; no restore path).
func TestPhysics2D_CardinalIntegration(t *testing.T) {
	// Not parallel: uses package-level harness state and testRequire for the verify system.
	t.Setenv("LOG_LEVEL", "disabled")
	resetHarnessGlobals()

	testRequire = require.New(t)
	t.Cleanup(func() {
		testRequire = nil
	})

	// Debug on: cardinal.Tick touches debug perf hooks (nil debug would panic).
	debug := true
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		Region:              "local",
		Organization:        "physics2d-e2e",
		Project:             "physics2d-e2e",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1_000_000,
		Debug:               &debug,
	})
	require.NoError(t, err)

	// Init hook must run before plugin Init so FullRebuildFromECS sees harness entities.
	cardinal.RegisterSystem(world, sceneInitSystem, cardinal.WithHook(cardinal.Init))
	cardinal.RegisterPlugin(world, physics.NewPlugin(physics.Config{
		Gravity:  physics.Vec2{X: 0, Y: -10},
		TickRate: 60,
	}))
	// Gameplay system moves the manual body each tick (before physics reconcile).
	cardinal.RegisterSystem(world, manualMoveSystem, cardinal.WithHook(cardinal.PreUpdate))
	// Assertions run after physics step (same-tick contact receivers).
	cardinal.RegisterSystem(world, verifySystem, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(world)

	ctx := context.Background()
	const lastTick = tickCrash2Verify + 5
	// Deterministic timestamps; loop count covers all scripted phases including post–crash 2 buffer.
	for i := range lastTick + 1 {
		world.Tick(ctx, time.Unix(int64(i), 0))
		if t.Failed() {
			t.Fatalf("failed at cardinal tick loop i=%d", i)
		}
	}
}
