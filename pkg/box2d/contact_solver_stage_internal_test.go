package box2d

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSolverStageContractFrictionRelaxOnly pins the push/relax stage split in
// solveContactsColor: the push stage (useBias=true) solves the normal
// constraint ONLY and must leave the friction (tangent) impulse at its
// prepared value, while the relax stage (useBias=false) applies friction.
//
// The contract is derived from the upstream v3.2.0 b2PushContacts_Overflow
// (push: normal only) / b2SolveContacts_Overflow (relax: normal + friction +
// rolling) split that this port now follows. The previous solver applied
// friction in both stages, so on that solver the push call leaves a nonzero
// tangentImpulse and this test fails.
func TestSolverStageContractFrictionRelaxOnly(t *testing.T) {
	def := DefaultWorldDef()
	def.EnableSleep = false
	w := NewWorld(&def)
	t.Cleanup(w.Destroy)

	gd := DefaultBodyDef()
	gd.Position = Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	gbox := MakeBox(50.0, 10.0)
	gsd := DefaultShapeDef()
	gsd.Material.Friction = 0.5
	w.CreatePolygonShape(ground, &gsd, &gbox)

	// Sliding box: nonzero tangent velocity so the relax stage produces a
	// nonzero friction impulse.
	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: 0.5}
	bd.LinearVelocity = Vec2{X: 5.0, Y: 0.0}
	box := w.CreateBody(&bd)
	bshape := MakeBox(0.5, 0.5)
	sd := DefaultShapeDef()
	sd.Density = 1.0
	sd.Material.Friction = 0.5
	w.CreatePolygonShape(box, &sd, &bshape)

	// One Step registers the contact in the constraint graph. A fresh
	// contact's manifold impulses start at zero, but the step commits
	// nonzero impulses; disabling warm starting makes re-preparing zero the
	// cached impulses (prepareContactsColor scales them by warmStartScale),
	// isolating the stage calls.
	stepWorld(w, 1)
	w.EnableWarmStarting(false)

	ctx := newStageTestCtx(w)

	constraints, colorIndex := prepareStageConstraints(t, w, ctx)

	// Push stage: normal only — tangent impulse must stay at zero.
	w.solveContactsColor(ctx, colorIndex, true, 0, len(constraints))
	for i := range constraints {
		c := &constraints[i]
		require.Zero(t, c.rollingImpulse, "push must not apply rolling resistance")
		for j := range c.points[:c.pointCount] {
			require.Zero(t, c.points[j].tangentImpulse,
				"push stage must not apply friction (point %d)", j)
		}
	}

	// Relax stage: friction is applied — tangent impulse becomes nonzero.
	w.solveContactsColor(ctx, colorIndex, false, 0, len(constraints))
	anyTangent := false
	for i := range constraints {
		c := &constraints[i]
		for j := range c.points[:c.pointCount] {
			if c.points[j].tangentImpulse != 0.0 {
				anyTangent = true
				break
			}
		}
		if anyTangent {
			break
		}
	}
	require.True(t, anyTangent, "relax stage must apply friction")
}

// TestSolverStageContractRollingResistanceRelaxOnly pins the relax-only
// rolling-resistance half of the stage split: the push stage (useBias=true)
// must leave the rolling impulse at its prepared value, while the relax
// stage (useBias=false) applies rolling resistance. Fails on the previous
// solver, which applied rolling resistance in both stages.
func TestSolverStageContractRollingResistanceRelaxOnly(t *testing.T) {
	def := DefaultWorldDef()
	def.EnableSleep = false
	w := NewWorld(&def)
	t.Cleanup(w.Destroy)

	gd := DefaultBodyDef()
	gd.Position = Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	gbox := MakeBox(50.0, 10.0)
	gsd := DefaultShapeDef()
	gsd.Material.Friction = 0.6
	gsd.Material.RollingResistance = 0.3
	w.CreatePolygonShape(ground, &gsd, &gbox)

	// Ball with initial spin so wB-wA != 0 and the relax stage produces a
	// nonzero rolling impulse.
	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: 0.5}
	bd.LinearVelocity = Vec2{X: 3.0, Y: 0.0}
	bd.AngularVelocity = 10.0
	ball := w.CreateBody(&bd)
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	sd := DefaultShapeDef()
	sd.Material.Friction = 0.6
	sd.Material.RollingResistance = 0.3
	w.CreateCircleShape(ball, &sd, &circle)

	stepWorld(w, 1)
	w.EnableWarmStarting(false)

	ctx := newStageTestCtx(w)
	constraints, colorIndex := prepareStageConstraints(t, w, ctx)

	// Push stage: rolling resistance must not be applied.
	w.solveContactsColor(ctx, colorIndex, true, 0, len(constraints))
	for i := range constraints {
		require.Zero(t, constraints[i].rollingImpulse,
			"push stage must not apply rolling resistance (constraint %d)", i)
	}

	// Relax stage: rolling resistance is applied.
	w.solveContactsColor(ctx, colorIndex, false, 0, len(constraints))
	anyRolling := false
	for i := range constraints {
		if constraints[i].rollingImpulse != 0.0 {
			anyRolling = true
			break
		}
	}
	require.True(t, anyRolling, "relax stage must apply rolling resistance")
}

// newStageTestCtx builds a stepContext matching the real solver's per-sub-step
// softness and awake-set state (mirrors TestPreparedConstraintSoftness). Only
// the fields read by prepareContactsColor/solveContactsColor are populated.
func newStageTestCtx(w *World) *stepContext {
	ctx := &stepContext{}
	ctx.dt = 1.0 / 60.0
	ctx.subStepCount = 4
	ctx.invDT = 1.0 / ctx.dt
	ctx.h = ctx.dt / float64(ctx.subStepCount)
	ctx.invH = float64(ctx.subStepCount) * ctx.invDT
	contactHertz := minFloat(w.contactHertz, 0.125*ctx.invH)
	ctx.contactSoftness = makeSoft(contactHertz, w.contactDampingRatio, ctx.h)
	ctx.staticSoftness = makeSoft(2.0*contactHertz, w.contactDampingRatio, ctx.h)
	ctx.graph = &w.constraintGraph
	ctx.states = (&w.solverSets[awakeSet]).bodyStates
	return ctx
}

// prepareStageConstraints allocates and prepares the contact constraints for
// the single color holding the box/ball-ground contact, and returns those
// constraints together with their color index.
func prepareStageConstraints(t *testing.T, w *World, ctx *stepContext) ([]contactConstraint, int) {
	t.Helper()

	for colorIndex := range GraphColorCount {
		color := &w.constraintGraph.colors[colorIndex]
		count := len(color.contactSims)
		if count == 0 {
			continue
		}
		constraints := make([]contactConstraint, count)
		color.constraints = constraints
		w.prepareContactsColor(ctx, colorIndex, 0, count)
		return constraints, colorIndex
	}
	t.Fatal("no contact constraints found in any color")
	return nil, -1
}
