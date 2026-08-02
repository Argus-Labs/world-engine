// Oracle tests for world lifecycle, callbacks and locked-world semantics.
//
// Every expectation in this file is derived from the vendored C source of
// truth (box2d/src at v3.2.0-era vendor) or from upstream test_world.c and
// docs/simulation.md — never from running the Go port. C citations are given
// as file:line next to each nontrivial assertion.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const (
	wboDT       = 1.0 / 60.0
	wboSubSteps = 4
)

// wboNewWorld creates a world with upstream default gravity {0,-10}
// (test_world.c HelloWorld) and registers cleanup.
func wboNewWorld(t *testing.T) *box2d.World {
	t.Helper()

	def := box2d.DefaultWorldDef()
	def.Gravity = box2d.Vec2{X: 0.0, Y: -10.0}
	w := box2d.NewWorld(&def)
	t.Cleanup(w.Destroy)
	return w
}

// wboGround creates the standard static ground: a 100x20 box whose top
// surface is the line y = 0. Returns the ground shape id.
func wboGround(t *testing.T, w *box2d.World) (box2d.BodyID, box2d.ShapeID) {
	t.Helper()

	bd := box2d.DefaultBodyDef()
	bd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&bd)

	groundBox := box2d.MakeBox(50.0, 10.0)
	sd := box2d.DefaultShapeDef()
	shapeID := w.CreatePolygonShape(ground, &sd, &groundBox)
	return ground, shapeID
}

// wboDynamicBox creates a dynamic unit box (MakeBox(0.5,0.5), density 1).
func wboDynamicBox(t *testing.T, w *box2d.World, pos box2d.Vec2) (box2d.BodyID, box2d.ShapeID) {
	t.Helper()

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = pos
	body := w.CreateBody(&bd)

	box := box2d.MakeBox(0.5, 0.5)
	sd := box2d.DefaultShapeDef()
	shapeID := w.CreatePolygonShape(body, &sd, &box)
	return body, shapeID
}

// TestOracleEmptyWorld ports EmptyWorld (test_world.c:101-121): stepping an
// empty world 60 times must be a no-op and destroying the world must
// invalidate its id.
func TestOracleEmptyWorld(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultWorldDef()
	w := box2d.NewWorld(&def)
	id := w.ID()
	require.True(t, w.IsWorldValid(id))

	for range 60 {
		w.Step(wboDT, 1)
	}

	w.Destroy()
	require.False(t, w.IsWorldValid(id))
}

// TestOracleDestroyAllBodies ports DestroyAllBodiesWorld
// (test_world.c:123-171): interleave creation and destruction of dynamic
// bodies with steps; the final body count must be zero.
func TestOracleDestroyAllBodies(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultWorldDef()
	w := box2d.NewWorld(&def)

	const bodyCount = 10
	var bodyIDs [bodyCount]box2d.BodyID
	count := 0
	creating := true

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	square := box2d.MakeBox(0.5, 0.5)

	for range 2*bodyCount + 10 {
		switch {
		case creating && count < bodyCount:
			bodyIDs[count] = w.CreateBody(&bd)
			sd := box2d.DefaultShapeDef()
			w.CreatePolygonShape(bodyIDs[count], &sd, &square)
			count++
		case creating:
			creating = false
		case count > 0:
			w.DestroyBody(bodyIDs[count-1])
			count--
		}

		w.Step(wboDT, 3)
	}

	// test_world.c:164: counters.bodyCount == 0.
	counters := w.Counters()
	require.Equal(t, 0, counters.BodyCount)

	id := w.ID()
	w.Destroy()
	require.False(t, w.IsWorldValid(id))
}

// TestOracleIsValid ports TestIsValid (test_world.c:174-202): body ids go
// invalid on body destruction and on world destruction.
func TestOracleIsValid(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultWorldDef()
	w := box2d.NewWorld(&def)
	require.True(t, w.IsWorldValid(w.ID()))

	bd := box2d.DefaultBodyDef()
	body1 := w.CreateBody(&bd)
	require.True(t, w.IsBodyValid(body1))

	body2 := w.CreateBody(&bd)
	require.True(t, w.IsBodyValid(body2))

	w.DestroyBody(body1)
	require.False(t, w.IsBodyValid(body1))

	w.DestroyBody(body2)
	require.False(t, w.IsBodyValid(body2))

	id := w.ID()
	w.Destroy()
	require.False(t, w.IsWorldValid(id))
	require.False(t, w.IsBodyValid(body1))
	require.False(t, w.IsBodyValid(body2))
}

// TestOracleWorldCoverage ports TestWorldCoverage (test_world.c:266-327):
// every world setter/getter pair must round-trip exactly as upstream asserts.
func TestOracleWorldCoverage(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultWorldDef()
	w := box2d.NewWorld(&def)
	t.Cleanup(w.Destroy)

	w.EnableSleeping(true)
	w.EnableSleeping(false)
	require.False(t, w.IsSleepingEnabled())

	w.EnableContinuous(false)
	w.EnableContinuous(true)
	require.True(t, w.IsContinuousEnabled())

	w.SetRestitutionThreshold(0.0)
	w.SetRestitutionThreshold(2.0)
	require.InDelta(t, 2.0, w.RestitutionThreshold(), 0.0)

	w.SetHitEventThreshold(0.0)
	w.SetHitEventThreshold(100.0)
	require.InDelta(t, 100.0, w.HitEventThreshold(), 0.0)

	// test_world.c:293-294: register callbacks; upstream only checks linkage.
	w.SetCustomFilterCallback(func(_, _ box2d.ShapeID, ctx any) bool {
		require.Nil(t, ctx)
		return true
	}, nil)
	w.SetPreSolveCallback(func(_, _ box2d.ShapeID, _, _ box2d.Vec2, ctx any) bool {
		require.Nil(t, ctx)
		return false
	}, nil)

	// physics_world.c b2World_SetFrictionCallback/SetRestitutionCallback:
	// passing NULL restores the default mixing rules.
	w.SetFrictionCallback(func(fa float64, _ uint64, fb float64, _ uint64) float64 {
		return fa + fb
	})
	w.SetFrictionCallback(nil)
	w.SetRestitutionCallback(func(ra float64, _ uint64, rb float64, _ uint64) float64 {
		return ra + rb
	})
	w.SetRestitutionCallback(nil)

	g := box2d.Vec2{X: 1.0, Y: 2.0}
	w.SetGravity(g)
	require.InDelta(t, g.X, w.Gravity().X, 0.0)
	require.InDelta(t, g.Y, w.Gravity().Y, 0.0)

	explosionDef := box2d.DefaultExplosionDef()
	w.Explode(&explosionDef)

	w.SetContactTuning(10.0, 2.0, 4.0)

	w.SetMaximumLinearSpeed(10.0)
	require.InDelta(t, 10.0, w.MaximumLinearSpeed(), 0.0)

	w.EnableWarmStarting(true)
	require.True(t, w.IsWarmStartingEnabled())

	require.Equal(t, 0, w.AwakeBodyCount())

	w.SetUserData(42)
	require.Equal(t, uint64(42), w.UserData())

	w.Step(1.0, 1)
}

// TestOracleContactTuningAndRecycleDistance covers the unlocked paths of
// b2World_SetContactTuning and b2World_SetContactRecycleDistance
// (physics_world.c): the recycle distance must round-trip and a scene must
// keep settling with modified tuning.
func TestOracleContactTuningAndRecycleDistance(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	wboGround(t, w)

	w.SetContactRecycleDistance(2.5)
	require.InDelta(t, 2.5, w.ContactRecycleDistance(), 0.0)

	w.SetContactTuning(45.0, 5.0, 2.0)

	body, _ := wboDynamicBox(t, w, box2d.Vec2{X: 0.0, Y: 1.0})
	for range 120 {
		w.Step(wboDT, wboSubSteps)
	}

	// Analytic rest height: ground top (y=0) + half extent (0.5), within a
	// few linear slops (core.h B2_LINEAR_SLOP scale).
	require.InDelta(t, 0.5, w.BodyPosition(body).Y, 4.0*box2d.LinearSlop)
}

// TestOracleLockedWorldIgnoresCalls verifies the locked-world guards. The
// vendored C returns early (or returns zero values) from mutators and event
// getters while the world is stepping: physics_world.c:764-765 (b2World_Step
// reentry) and the `if (world->locked) return;` guards in every setter, and
// body.c mirrors them for the body mutators. The hook into the locked state
// is the pre-solve callback, which runs inside Step (contact.c pre-solve
// branch in b2UpdateContact).
func TestOracleLockedWorldIgnoresCalls(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	wboGround(t, w)

	// Resting box with pre-solve events so the callback fires on step one.
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 0.5}
	body := w.CreateBody(&bd)
	box := box2d.MakeBox(0.5, 0.5)
	sd := box2d.DefaultShapeDef()
	sd.EnablePreSolveEvents = true
	w.CreatePolygonShape(body, &sd, &box)

	// A pre-disabled body: EnableBody while locked must be ignored.
	bd2 := box2d.DefaultBodyDef()
	bd2.Type = box2d.DynamicBody
	bd2.Position = box2d.Vec2{X: 20.0, Y: 0.5}
	disabledBody := w.CreateBody(&bd2)
	w.DisableBody(disabledBody)
	require.False(t, w.IsBodyEnabled(disabledBody))

	preLinearDamping := w.BodyLinearDamping(body)
	preAngularDamping := w.BodyAngularDamping(body)
	preGravityScale := w.BodyGravityScale(body)

	fired := false
	w.SetPreSolveCallback(func(_, _ box2d.ShapeID, _, _ box2d.Vec2, _ any) bool {
		if fired {
			return true
		}
		fired = true

		// physics_world.c:764-765: reentrant b2World_Step returns early.
		w.Step(wboDT, wboSubSteps)

		// Event getters return zero values while locked.
		require.Empty(t, w.BodyEvents().MoveEvents)
		require.Empty(t, w.SensorEvents().BeginEvents)
		require.Empty(t, w.ContactEvents().BeginEvents)
		require.Empty(t, w.JointEvents().JointEvents)

		// World mutators are ignored while locked.
		w.EnableSleeping(false)
		w.EnableWarmStarting(false)
		w.EnableContinuous(false)
		w.SetRestitutionThreshold(9.0)
		w.SetHitEventThreshold(9.0)
		w.SetContactRecycleDistance(9.0)
		w.SetMaximumLinearSpeed(9.0)
		w.SetContactTuning(60.0, 5.0, 5.0)
		w.SetFrictionCallback(func(_ float64, _ uint64, _ float64, _ uint64) float64 {
			return 0.0
		})
		w.SetRestitutionCallback(func(_ float64, _ uint64, _ float64, _ uint64) float64 {
			return 0.0
		})

		// Body mutators are ignored while locked (body.c locked guards).
		w.DisableBody(body)
		w.EnableBody(disabledBody)
		w.DestroyBody(body)
		w.ApplyBodyMassFromShapes(body)
		w.SetBodyLinearDamping(body, 9.0)
		w.SetBodyAngularDamping(body, 9.0)
		w.SetBodyGravityScale(body, 9.0)
		w.SetBodyAwake(body, false)
		w.EnableBodySleep(body, false)
		w.SetBodyMassData(body, box2d.MassData{Mass: 9.0, RotationalInertia: 9.0})
		w.SetBodyMotionLocks(body, box2d.MotionLocks{LinearX: true})

		// Body getters return zero values while locked
		// (body.c b2Body_GetContactCapacity / GetContactData / ComputeAABB).
		require.Equal(t, 0, w.BodyContactCapacity(body))
		require.Equal(t, 0, w.BodyContactData(body, make([]box2d.ContactData, 4)))
		aabb := w.ComputeBodyAABB(body)
		require.InDelta(t, 0.0, aabb.LowerBound.X, 0.0)
		require.InDelta(t, 0.0, aabb.UpperBound.Y, 0.0)

		return true
	}, nil)

	w.Step(wboDT, wboSubSteps)
	require.True(t, fired, "pre-solve callback must fire for the resting contact")

	// All locked mutations must have been dropped.
	require.True(t, w.IsSleepingEnabled())
	require.True(t, w.IsWarmStartingEnabled())
	require.True(t, w.IsContinuousEnabled())
	require.InDelta(t, box2d.DefaultWorldDef().RestitutionThreshold, w.RestitutionThreshold(), 0.0)
	require.InDelta(t, box2d.DefaultWorldDef().HitEventThreshold, w.HitEventThreshold(), 0.0)
	require.InDelta(t, box2d.ContactRecycleDistance, w.ContactRecycleDistance(), 0.0)
	require.InDelta(t, box2d.DefaultWorldDef().MaximumLinearSpeed, w.MaximumLinearSpeed(), 0.0)

	require.True(t, w.IsBodyValid(body), "locked DestroyBody must be ignored")
	require.True(t, w.IsBodyEnabled(body), "locked DisableBody must be ignored")
	require.False(t, w.IsBodyEnabled(disabledBody), "locked EnableBody must be ignored")
	require.True(t, w.IsBodySleepEnabled(body))
	require.InDelta(t, preLinearDamping, w.BodyLinearDamping(body), 0.0)
	require.InDelta(t, preAngularDamping, w.BodyAngularDamping(body), 0.0)
	require.InDelta(t, preGravityScale, w.BodyGravityScale(body), 0.0)
	locks := w.BodyMotionLocks(body)
	require.False(t, locks.LinearX)
}

// TestOracleCustomFilterDisablesCollision: broad-phase pair creation asks the
// custom filter when a shape opts in; returning false suppresses the pair so
// the shapes never collide, including the continuous sweep
// (physics_world.c b2World_SetCustomFilterCallback; broad_phase pair update).
func TestOracleCustomFilterDisablesCollision(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	bd := box2d.DefaultBodyDef()
	bd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&bd)
	groundBox := box2d.MakeBox(50.0, 10.0)
	gsd := box2d.DefaultShapeDef()
	gsd.EnableCustomFiltering = true
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	bd2 := box2d.DefaultBodyDef()
	bd2.Type = box2d.DynamicBody
	bd2.Position = box2d.Vec2{X: 0.0, Y: 1.0}
	faller := w.CreateBody(&bd2)
	box := box2d.MakeBox(0.5, 0.5)
	fsd := box2d.DefaultShapeDef()
	fsd.EnableCustomFiltering = true
	w.CreatePolygonShape(faller, &fsd, &box)

	calls := 0
	w.SetCustomFilterCallback(func(_, _ box2d.ShapeID, _ any) bool {
		calls++
		return false
	}, nil)

	for range 240 {
		w.Step(wboDT, wboSubSteps)
	}

	require.Positive(t, calls, "custom filter must be consulted")
	require.Less(t, w.BodyPosition(faller).Y, -5.0,
		"filtered-out pair must not collide; the box must fall through the ground")
}

// TestOraclePreSolveDisablesContact: when the pre-solve callback returns
// false the manifold is cleared and the contact does not generate constraints
// (contact.c b2UpdateContact pre-solve branch; upstream PreSolveStatic in
// test_world.c:253 returns false).
func TestOraclePreSolveDisablesContact(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	wboGround(t, w)

	// Continuous is disabled so only the (disabled) discrete contact could
	// stop the box.
	w.EnableContinuous(false)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 1.0}
	faller := w.CreateBody(&bd)
	box := box2d.MakeBox(0.5, 0.5)
	sd := box2d.DefaultShapeDef()
	sd.EnablePreSolveEvents = true
	w.CreatePolygonShape(faller, &sd, &box)

	calls := 0
	w.SetPreSolveCallback(func(_, _ box2d.ShapeID, _, _ box2d.Vec2, _ any) bool {
		calls++
		return false
	}, nil)

	for range 240 {
		w.Step(wboDT, wboSubSteps)
	}

	require.Positive(t, calls, "pre-solve callback must fire for the touching manifold")
	require.Less(t, w.BodyPosition(faller).Y, -5.0,
		"pre-solve false must disable the contact; the box must fall through")
}

// TestOracleFrictionCallbackOverride: b2UpdateContact recomputes the mixed
// friction through world->frictionCallback each update (contact.c). A
// callback returning zero removes all tangential resistance, so a sliding box
// keeps its speed while the default mix (sqrt(fA*fB), types.c
// b2DefaultFrictionCallback) stops it.
func TestOracleFrictionCallbackOverride(t *testing.T) {
	t.Parallel()

	slideSpeed := func(t *testing.T, zeroFriction bool) float64 {
		t.Helper()

		w := wboNewWorld(t)
		wboGround(t, w)

		if zeroFriction {
			w.SetFrictionCallback(func(_ float64, _ uint64, _ float64, _ uint64) float64 {
				return 0.0
			})
		}

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: 0.0, Y: 0.5}
		bd.LinearVelocity = box2d.Vec2{X: 5.0, Y: 0.0}
		slider := w.CreateBody(&bd)
		box := box2d.MakeBox(0.5, 0.5)
		sd := box2d.DefaultShapeDef()
		w.CreatePolygonShape(slider, &sd, &box)

		for range 90 {
			w.Step(wboDT, wboSubSteps)
		}

		return w.BodyLinearVelocity(slider).X
	}

	// Default friction 0.6 both sides: deceleration ~ mu*g = 6 m/s^2 stops
	// the box well within 1.5 s.
	defaultSpeed := slideSpeed(t, false)
	require.Less(t, math.Abs(defaultSpeed), 0.5)

	// Zero friction: no tangential impulse, the box keeps sliding.
	freeSpeed := slideSpeed(t, true)
	require.Greater(t, freeSpeed, 4.5)
}

// TestOracleRestitutionCallbackOverride: b2UpdateContact recomputes the mixed
// restitution through world->restitutionCallback (contact.c); the default mix
// is max(rA, rB) (types.c b2DefaultRestitutionCallback). Overriding to 0.9
// must make a nearly dead ball bounce: rebound speed ~ e * impact speed
// (contact_solver.c restitution application above the threshold).
func TestOracleRestitutionCallbackOverride(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	wboGround(t, w)

	w.SetRestitutionCallback(func(_ float64, _ uint64, _ float64, _ uint64) float64 {
		return 0.9
	})

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 2.5}
	ball := w.CreateBody(&bd)
	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
	sd := box2d.DefaultShapeDef()
	sd.Material.Restitution = 0.05
	w.CreateCircleShape(ball, &sd, &circle)

	// Track the peak height reached after the first bounce. Analytic bound:
	// impact speed sqrt(2*g*2) = 6.32, rebound 0.9*6.32 = 5.69, rise
	// v^2/(2g) = 1.62 above the rest height 0.5. Solver losses shrink this,
	// so require a peak clearly above what restitution 0.05 could produce.
	bounced := false
	peak := 0.0
	for range 600 {
		w.Step(wboDT, wboSubSteps)
		vy := w.BodyLinearVelocity(ball).Y
		y := w.BodyPosition(ball).Y
		if vy > 0.1 {
			bounced = true
		}
		if bounced {
			peak = math.Max(peak, y)
		}
	}

	require.True(t, bounced, "ball must rebound with restitution callback 0.9")
	require.Greater(t, peak, 1.2)
}

// TestOracleRollingResistanceSlowsSpin: b2UpdateContact scales rolling
// resistance by the max shape radius when either material sets it
// (contact.c rolling-resistance branch); the solver then applies a resisting
// rolling impulse. A rolling ball with rolling resistance must lose angular
// speed relative to an identical ball without it.
func TestOracleRollingResistanceSlowsSpin(t *testing.T) {
	t.Parallel()

	spinAfter := func(t *testing.T, rollingResistance float64) float64 {
		t.Helper()

		w := wboNewWorld(t)
		wboGround(t, w)
		w.EnableSleeping(false)

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: 0.0, Y: 0.5}
		bd.LinearVelocity = box2d.Vec2{X: 3.0, Y: 0.0}
		ball := w.CreateBody(&bd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
		sd := box2d.DefaultShapeDef()
		sd.Material.RollingResistance = rollingResistance
		w.CreateCircleShape(ball, &sd, &circle)

		for range 300 {
			w.Step(wboDT, wboSubSteps)
		}

		return math.Abs(w.BodyAngularVelocity(ball))
	}

	freeSpin := spinAfter(t, 0.0)
	dampedSpin := spinAfter(t, 0.3)

	// Friction spins the free ball up towards rolling (w ~ v/r); rolling
	// resistance must bleed that spin off.
	require.Greater(t, freeSpin, 1.0)
	require.Less(t, dampedSpin, 0.5*freeSpin)
}

// TestOracleSleepingSetMergeOnJointCreate: creating a joint between bodies in
// two different sleeping solver sets merges the sets and both bodies stay
// asleep (solver_set.c b2MergeSolverSets, called from joint creation). Waking
// one body afterwards wakes the merged set.
func TestOracleSleepingSetMergeOnJointCreate(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	makeFloater := func(x float64) box2d.BodyID {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: x, Y: 0.0}
		bd.GravityScale = 0.0
		body := w.CreateBody(&bd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
		sd := box2d.DefaultShapeDef()
		w.CreateCircleShape(body, &sd, &circle)
		return body
	}

	bodyA := makeFloater(0.0)
	bodyB := makeFloater(3.0)

	// Put each single-body island to sleep: two distinct sleeping sets.
	w.SetBodyAwake(bodyA, false)
	w.SetBodyAwake(bodyB, false)
	require.False(t, w.IsBodyAwake(bodyA))
	require.False(t, w.IsBodyAwake(bodyB))

	def := box2d.DefaultDistanceJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	def.Length = 3.0
	joint := w.CreateDistanceJoint(&def)
	require.True(t, w.IsJointValid(joint))

	// solver_set.c: the merge keeps both sets asleep.
	require.False(t, w.IsBodyAwake(bodyA), "merged sleeping sets must stay asleep")
	require.False(t, w.IsBodyAwake(bodyB), "merged sleeping sets must stay asleep")

	for range 5 {
		w.Step(wboDT, wboSubSteps)
	}
	require.False(t, w.IsBodyAwake(bodyA))
	require.False(t, w.IsBodyAwake(bodyB))

	// Waking one body wakes the whole merged solver set.
	w.SetBodyAwake(bodyA, true)
	require.True(t, w.IsBodyAwake(bodyA))
	require.True(t, w.IsBodyAwake(bodyB))

	// Second merge with asymmetric set sizes: solver_set.c moves the fewest
	// bodies, swapping the sets when the first one is smaller, and transfers
	// the joints stored in the moved set.
	w.SetBodyAwake(bodyA, false)
	require.False(t, w.IsBodyAwake(bodyA))
	require.False(t, w.IsBodyAwake(bodyB))

	bodyC := makeFloater(6.0)
	w.SetBodyAwake(bodyC, false)
	require.False(t, w.IsBodyAwake(bodyC))

	// BodyIDA is the single-body set: setID1 is smaller than setID2 (the
	// two-body set holding the A-B joint), so the merge swaps them.
	def2 := box2d.DefaultDistanceJointDef()
	def2.Base.BodyIDA = bodyC
	def2.Base.BodyIDB = bodyB
	def2.Length = 3.0
	joint2 := w.CreateDistanceJoint(&def2)
	require.True(t, w.IsJointValid(joint2))

	require.False(t, w.IsBodyAwake(bodyA))
	require.False(t, w.IsBodyAwake(bodyB))
	require.False(t, w.IsBodyAwake(bodyC))

	w.SetBodyAwake(bodyC, true)
	require.True(t, w.IsBodyAwake(bodyA))
	require.True(t, w.IsBodyAwake(bodyB))
	require.True(t, w.IsBodyAwake(bodyC))

	// Third merge: the moved (smaller) set carries a joint of its own, so
	// the merge also transfers joint sims (solver_set.c joint transfer loop).
	w.SetBodyAwake(bodyA, false) // sleeps the A-B-C island (3 bodies, 2 joints)

	bodyD := makeFloater(9.0)
	bodyE := makeFloater(12.0)
	def3 := box2d.DefaultDistanceJointDef()
	def3.Base.BodyIDA = bodyD
	def3.Base.BodyIDB = bodyE
	def3.Length = 3.0
	joint3 := w.CreateDistanceJoint(&def3)
	require.True(t, w.IsJointValid(joint3))
	w.SetBodyAwake(bodyD, false) // sleeps the D-E island (2 bodies, 1 joint)
	require.False(t, w.IsBodyAwake(bodyE))

	def4 := box2d.DefaultDistanceJointDef()
	def4.Base.BodyIDA = bodyE
	def4.Base.BodyIDB = bodyA
	def4.Length = 12.0
	joint4 := w.CreateDistanceJoint(&def4)
	require.True(t, w.IsJointValid(joint4))

	for _, body := range []box2d.BodyID{bodyA, bodyB, bodyC, bodyD, bodyE} {
		require.False(t, w.IsBodyAwake(body), "all merged bodies must stay asleep")
	}

	w.SetBodyAwake(bodyA, true)
	for _, body := range []box2d.BodyID{bodyA, bodyB, bodyC, bodyD, bodyE} {
		require.True(t, w.IsBodyAwake(body), "waking one body wakes the merged set")
	}

	// Fourth merge: the moved set carries a touching contact, so the merge
	// also transfers contact sims (solver_set.c contact transfer loop).
	w.SetBodyAwake(bodyA, false) // sleeps the joined 5-body island

	// Two circles with a 0.01 gap: within the speculative distance, so the
	// manifold has a point and the contact counts as touching without any
	// pushout (manifold.c b2CollideCircles; contact.c touching flag).
	bodyF := makeFloater(20.0)
	bodyG := makeFloater(21.01)
	w.Step(wboDT, wboSubSteps)
	require.True(t, w.IsBodyAwake(bodyF))

	w.SetBodyAwake(bodyF, false) // sleeps the F-G island (2 bodies, 1 contact)
	require.False(t, w.IsBodyAwake(bodyG))

	def5 := box2d.DefaultDistanceJointDef()
	def5.Base.BodyIDA = bodyF
	def5.Base.BodyIDB = bodyA
	def5.Length = 20.0
	joint5 := w.CreateDistanceJoint(&def5)
	require.True(t, w.IsJointValid(joint5))

	for _, body := range []box2d.BodyID{bodyA, bodyB, bodyC, bodyD, bodyE, bodyF, bodyG} {
		require.False(t, w.IsBodyAwake(body), "all merged bodies must stay asleep")
	}

	w.SetBodyAwake(bodyG, true)
	for _, body := range []box2d.BodyID{bodyA, bodyB, bodyC, bodyD, bodyE, bodyF, bodyG} {
		require.True(t, w.IsBodyAwake(body), "waking one body wakes the merged set")
	}
}

// TestOracleSpeculativeDisabledStillSettles: b2World_EnableSpeculative is an
// upstream testing flag; with it off, two-point manifolds with large
// separation get trimmed (contact.c) yet a box must still settle at the
// analytic rest height.
func TestOracleSpeculativeDisabledStillSettles(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	wboGround(t, w)
	w.EnableSpeculative(false)

	body, _ := wboDynamicBox(t, w, box2d.Vec2{X: 0.0, Y: 1.0})
	for range 180 {
		w.Step(wboDT, wboSubSteps)
	}

	require.InDelta(t, 0.5, w.BodyPosition(body).Y, 4.0*box2d.LinearSlop)
	require.Less(t, math.Abs(w.BodyLinearVelocity(body).Y), 0.05)
}

// TestOracleRebuildStaticTree: b2World_RebuildStaticTree rebuilds the static
// broad-phase tree in place (physics_world.c); resting contacts must survive
// the rebuild.
func TestOracleRebuildStaticTree(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	// A row of static unit boxes whose tops form the line y = 0.
	for i := range 10 {
		bd := box2d.DefaultBodyDef()
		bd.Position = box2d.Vec2{X: float64(2*i) - 9.0, Y: -0.5}
		ground := w.CreateBody(&bd)
		box := box2d.MakeBox(1.0, 0.5)
		sd := box2d.DefaultShapeDef()
		w.CreatePolygonShape(ground, &sd, &box)
	}

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 1.0, Y: 0.5}
	ball := w.CreateBody(&bd)
	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
	sd := box2d.DefaultShapeDef()
	w.CreateCircleShape(ball, &sd, &circle)

	for range 60 {
		w.Step(wboDT, wboSubSteps)
	}

	w.RebuildStaticTree()

	for range 60 {
		w.Step(wboDT, wboSubSteps)
	}

	require.InDelta(t, 0.5, w.BodyPosition(ball).Y, 4.0*box2d.LinearSlop)
}

// TestOracleProfileAndCounters: b2World_GetProfile and b2World_GetCounters
// report simulation statistics (physics_world.c). This port keeps the timing
// fields at zero deterministically; the counters must match the scene.
func TestOracleProfileAndCounters(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	wboGround(t, w)
	wboDynamicBox(t, w, box2d.Vec2{X: 0.0, Y: 0.5})
	wboDynamicBox(t, w, box2d.Vec2{X: 0.0, Y: 1.5})

	for range 10 {
		w.Step(wboDT, wboSubSteps)
	}

	profile := w.Profile()
	require.GreaterOrEqual(t, profile.Step, 0.0)
	require.GreaterOrEqual(t, profile.Solve, 0.0)

	counters := w.Counters()
	require.Equal(t, 3, counters.BodyCount)
	require.Equal(t, 3, counters.ShapeCount)
	require.GreaterOrEqual(t, counters.ContactCount, 2)
	require.GreaterOrEqual(t, counters.IslandCount, 1)
}
