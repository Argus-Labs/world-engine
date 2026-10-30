// Tests for the float64 port of Box2D v3.2.0 src/solver.c, src/solver.h
// (b2MakeSoft) and src/island.c (b2SplitIsland). Internal package tests
// because they inspect solver-set, island and constraint internals directly.

package box2d

import (
	"math"
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeSoft(t *testing.T) {
	// hertz == 0 disables the softness entirely. Bit comparisons keep the
	// checks exact (testifylint forbids Equal on floats).
	s := makeSoft(0.0, 10.0, 1.0/240.0)
	tassert.Equal(t, math.Float64bits(0.0), math.Float64bits(s.biasRate))
	tassert.Equal(t, math.Float64bits(0.0), math.Float64bits(s.massScale))
	tassert.Equal(t, math.Float64bits(0.0), math.Float64bits(s.impulseScale))

	// Default contact tuning: 30 Hz, zeta 10, quarter sub-step of 1/60.
	h := 1.0 / 240.0
	s = makeSoft(30.0, 10.0, h)

	// In all cases massScale + impulseScale == 1 (upstream comment).
	tassert.InDelta(t, 1.0, s.massScale+s.impulseScale, 1e-12)
	tassert.Positive(t, s.biasRate)
	tassert.Positive(t, s.massScale)
	tassert.Positive(t, s.impulseScale)

	// Exact algebra: biasRate = omega / a1, impulseScale = 1/(1+a2).
	omega := 2.0 * Pi * 30.0
	a1 := float64(2.0*10.0) + float64(h*omega)
	a2 := h * omega * a1
	tassert.Equal(t, math.Float64bits(omega/a1), math.Float64bits(s.biasRate))
	tassert.Equal(t, math.Float64bits(1.0/(1.0+a2)), math.Float64bits(s.impulseScale))
}

// stepWorld advances an internal-test world with the default step settings.
func stepWorld(w *World, steps int) {
	for range steps {
		w.Step(1.0/60.0, 4)
	}
}

// makeChainScene builds a static ground and three touching dynamic boxes so
// the contact chain forms a single island: A-B-C. Sleep is disabled so the
// solver never schedules an automatic split — the tests drive splitIsland
// directly.
func makeChainScene(t *testing.T) (*World, BodyID, BodyID, BodyID) {
	t.Helper()

	def := DefaultWorldDef()
	def.EnableSleep = false
	w := NewWorld(&def)
	t.Cleanup(w.Destroy)

	gd := DefaultBodyDef()
	gd.Position = Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	groundBox := MakeBox(50.0, 10.0)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	mk := func(x float64) BodyID {
		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: x, Y: 0.5}
		id := w.CreateBody(&bd)
		box := MakeBox(0.5, 0.5)
		sd := DefaultShapeDef()
		w.CreatePolygonShape(id, &sd, &box)
		return id
	}

	a := mk(-1.0)
	b := mk(0.0)
	c := mk(1.0)
	return w, a, b, c
}

func TestSplitIslandInternal(t *testing.T) {
	w, aID, bID, cID := makeChainScene(t)

	stepWorld(w, 30)

	bodyA := w.getBodyFullID(aID)
	bodyB := w.getBodyFullID(bID)
	bodyC := w.getBodyFullID(cID)

	// One island holding all three bodies.
	require.Equal(t, bodyA.islandID, bodyB.islandID)
	require.Equal(t, bodyB.islandID, bodyC.islandID)
	baseID := bodyA.islandID
	baseIsland := &w.islands[baseID]
	require.Len(t, baseIsland.bodies, 3)

	// Destroy the middle body's contacts by teleporting it away and letting
	// the broad phase drop the disjoint contacts.
	w.SetBodyTransform(bID, Vec2{X: 20.0, Y: 0.5}, MakeRot(0.0))
	stepWorld(w, 3)

	baseIsland = &w.islands[baseID]
	require.Positive(t, baseIsland.constraintRemoveCount, "removed contacts must be tracked")
	require.Equal(t, awakeSet, baseIsland.setIndex)

	// Split directly (the solver would otherwise do this when the bodies
	// become sleep candidates).
	w.splitIsland(baseID)

	// The base island is destroyed and each body owns a fresh island.
	tassert.Equal(t, NullIndex, w.islands[baseID].setIndex, "base island freed")

	bodyA = w.getBodyFullID(aID)
	bodyB = w.getBodyFullID(bID)
	bodyC = w.getBodyFullID(cID)

	ids := []int{bodyA.islandID, bodyB.islandID, bodyC.islandID}
	tassert.NotEqual(t, ids[0], ids[1])
	tassert.NotEqual(t, ids[1], ids[2])
	tassert.NotEqual(t, ids[0], ids[2])

	for i, body := range []*body{bodyA, bodyB, bodyC} {
		isl := &w.islands[ids[i]]
		require.Equal(t, awakeSet, isl.setIndex)
		require.Len(t, isl.bodies, 1)
		require.Equal(t, body.id, isl.bodies[0])
		require.Equal(t, 0, body.islandIndex)

		// Contacts against the static ground stay with the island of their
		// dynamic body.
		for _, link := range isl.contacts {
			linksBody := link.bodyIDA == body.id || link.bodyIDB == body.id
			require.True(t, linksBody, "island contact must reference the island body")
			require.Equal(t, ids[i], w.contacts[link.contactID].islandID)
		}
		require.Equal(t, 0, isl.constraintRemoveCount)
	}

	require.Equal(t, 3, getIDCount(&w.islandIDPool))

	// Arena scratch fully released.
	require.Equal(t, 0, getArenaAllocation(&w.arena))
}

// TestSplitIslandConnectedNoSplit checks the early-out: a still-connected
// island only clears its constraintRemoveCount.
func TestSplitIslandConnectedNoSplit(t *testing.T) {
	w, aID, bID, cID := makeChainScene(t)

	stepWorld(w, 30)

	bodyA := w.getBodyFullID(aID)
	baseID := bodyA.islandID

	// Fake a removed constraint without breaking connectivity.
	w.islands[baseID].constraintRemoveCount = 1

	w.splitIsland(baseID)

	bodyA = w.getBodyFullID(aID)
	bodyB := w.getBodyFullID(bID)
	bodyC := w.getBodyFullID(cID)

	tassert.Equal(t, baseID, bodyA.islandID, "island must survive")
	tassert.Equal(t, baseID, bodyB.islandID)
	tassert.Equal(t, baseID, bodyC.islandID)
	tassert.Equal(t, 0, w.islands[baseID].constraintRemoveCount)
	require.Equal(t, 0, getArenaAllocation(&w.arena))
}

// TestPreparedConstraintSoftness checks that contacts against static bodies
// receive the stiffer static softness while dynamic-vs-dynamic contacts use
// the regular contact softness (upstream b2PrepareOverflowContacts).
func TestPreparedConstraintSoftness(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	t.Cleanup(w.Destroy)

	gd := DefaultBodyDef()
	gd.Position = Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	groundBox := MakeBox(50.0, 10.0)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	// Two stacked dynamic boxes resting on the ground: one static contact
	// and one dynamic-dynamic contact.
	mk := func(y float64) {
		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: 0.0, Y: y}
		id := w.CreateBody(&bd)
		box := MakeBox(0.5, 0.5)
		sd := DefaultShapeDef()
		w.CreatePolygonShape(id, &sd, &box)
	}
	mk(0.5)
	mk(1.5)

	stepWorld(w, 10)

	// Rebuild the constraints exactly like solve does.
	ctx := stepContext{}
	ctx.world = w
	ctx.dt = 1.0 / 60.0
	ctx.subStepCount = 4
	ctx.invDT = 60.0
	ctx.h = ctx.dt / 4.0
	ctx.invH = 4.0 * ctx.invDT
	contactHertz := minFloat(w.contactHertz, 0.125*ctx.invH)
	ctx.contactSoftness = makeSoft(contactHertz, w.contactDampingRatio, ctx.h)
	ctx.staticSoftness = makeSoft(2.0*contactHertz, w.contactDampingRatio, ctx.h)
	ctx.graph = &w.constraintGraph
	awake := &w.solverSets[awakeSet]
	ctx.states = awake.bodyStates
	ctx.sims = awake.bodySims

	staticSeen := 0
	dynamicSeen := 0
	for colorIndex := range GraphColorCount {
		color := &w.constraintGraph.colors[colorIndex]
		count := len(color.contactSims)
		if count == 0 {
			continue
		}

		constraints := make([]contactConstraint, count)
		color.constraints = constraints
		w.prepareContactsColor(&ctx, colorIndex, 0, count)

		for i := range count {
			constraint := &constraints[i]
			if constraint.indexA == 0 || constraint.indexB == 0 {
				staticSeen++
				tassert.Equal(t, ctx.staticSoftness, constraint.softness)
			} else {
				dynamicSeen++
				tassert.Equal(t, ctx.contactSoftness, constraint.softness)
			}
		}

		color.constraints = nil
	}

	require.Positive(t, staticSeen, "expected at least one static contact")
	require.Positive(t, dynamicSeen, "expected at least one dynamic-dynamic contact")
}

// TestSolveClampsToMaxLinearSpeed checks the speed cap in
// integratePositionsTask (the post-solve clamp, upstream
// b2IntegratePositionsTask).
func TestSolveClampsToMaxLinearSpeed(t *testing.T) {
	def := DefaultWorldDef()
	def.MaximumLinearSpeed = 5.0
	w := NewWorld(&def)
	t.Cleanup(w.Destroy)

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: 100.0}
	bd.LinearVelocity = Vec2{X: 400.0, Y: 0.0}
	bd.GravityScale = 0.0
	body := w.CreateBody(&bd)
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	sd := DefaultShapeDef()
	w.CreateCircleShape(body, &sd, &circle)

	w.Step(1.0/60.0, 4)

	v := w.BodyLinearVelocity(body)
	tassert.LessOrEqual(t, Length(v), 5.0+1e-9, "velocity must be clamped to MaximumLinearSpeed")
}

// solverInducedDisplacement builds the deep-penetration scene used by
// TestPostSolveClampBoundsSolverInducedDisplacement and returns the vertical
// displacement of the dynamic box over one step. The world is zero-gravity, the
// box starts at rest, and it overlaps the static ground by 0.3 m, so every bit
// of motion this step is solver-induced by the contact penetration bias (no
// force, no gravity, no initial velocity).
func solverInducedDisplacement(t *testing.T, maxLinearSpeed float64) float64 {
	t.Helper()
	const dt = 1.0 / 60.0
	const subSteps = 4

	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	def.MaximumLinearSpeed = maxLinearSpeed
	def.EnableSleep = false
	w := NewWorld(&def)
	t.Cleanup(w.Destroy)

	// Static ground whose top surface sits at y = 0.
	gd := DefaultBodyDef()
	gd.Position = Vec2{X: 0.0, Y: -0.5}
	ground := w.CreateBody(&gd)
	groundBox := MakeBox(50.0, 0.5)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	// Dynamic half-extent box centered at y = 0.2 so its bottom is at
	// y = -0.3, i.e. 0.3 m of overlap with the ground. Zero gravity and zero
	// initial velocity: the only motion is the contact bias the solver imparts.
	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: 0.2}
	bd.LinearVelocity = Vec2Zero
	bd.GravityScale = 0.0
	body := w.CreateBody(&bd)
	box := MakeBox(0.5, 0.5)
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreatePolygonShape(body, &sd, &box)

	require.InDelta(t, 0.0, Length(w.BodyLinearVelocity(body)), 0.0,
		"the body must start at rest so all motion is solver-induced")

	start := w.BodyPosition(body)
	w.Step(dt, subSteps)
	end := w.BodyPosition(body)
	return end.Y - start.Y
}

// TestPostSolveClampBoundsSolverInducedDisplacement verifies that
// MaximumLinearSpeed bounds the velocity the constraint solve imparts and that
// integratePositionsTask integrates — the post-solve clamp placement (upstream
// v3.2.0 b2IntegratePositionsTask). Because the clamp runs after the solve and
// before position integration, the per-sub-step displacement cannot exceed
// maxLinearSpeed * h, so the whole-step displacement cannot exceed
// maxLinearSpeed * dt.
//
// With the old pre-solve placement (the v3.1.0 layout the port previously
// followed) the clamp capped only the velocity the solver RECEIVED, never the
// solver-induced velocity the integrator consumed, so this bound was absent:
// cap=1.0 and cap=400 produced bit-identical displacement ~2.67x over the
// bound. This test fails on that layout and passes once the clamp is restored
// to the post-solve position.
func TestPostSolveClampBoundsSolverInducedDisplacement(t *testing.T) {
	const maxSpeed = 1.0
	const dt = 1.0 / 60.0

	dispLow := solverInducedDisplacement(t, maxSpeed)

	// The within-step bound the post-solve clamp restores: the solver-induced
	// displacement over one step is bounded by maxLinearSpeed * dt.
	bound := maxSpeed * dt
	require.LessOrEqual(t, dispLow, bound+1e-9,
		"post-solve clamp must bound solver-induced displacement to cap*dt (%.6g), got %.6g", bound, dispLow)
	require.Positive(t, dispLow, "contact bias must push the penetrating body out of the ground")

	// The cap is actually binding on solver-induced motion: with the default
	// 400 m/s cap the bias velocity (~2.67 m/s) is far below it, so the body is
	// not clamped and displaces more than under the 1 m/s cap. Under the old
	// pre-solve clamp this was bit-identical to the capped case, so this
	// inequality is what distinguishes the two placements.
	dispHigh := solverInducedDisplacement(t, 400.0)
	require.Less(t, dispLow, dispHigh,
		"a low MaximumLinearSpeed must clamp solver-induced motion below the unclamped default")
}
