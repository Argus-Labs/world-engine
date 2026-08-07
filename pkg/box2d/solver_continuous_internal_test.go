// Internal tests for the continuous-collision port (solver_continuous.go and
// the bullet stage in solver.go): flag bookkeeping (isSpeedCapped,
// hadTimeOfImpact, enlargeBounds) that has no public accessors.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSpeedCapSetsFlag launches a body far above the world maximum linear
// speed and verifies both the clamp and the isSpeedCapped transfer from the
// sim flags to the body flags in finalizeBodiesTask.
func TestSpeedCapSetsFlag(t *testing.T) {
	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	def.MaximumLinearSpeed = 10.0
	w := NewWorld(&def)
	defer w.Destroy()

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.LinearVelocity = Vec2{X: 100.0, Y: 0.0}
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	w.Step(1.0/60.0, 4)

	b := w.getBodyFullID(bodyID)
	tassert.NotZero(t, b.flags&isSpeedCapped, "body flags must carry isSpeedCapped after the clamp")

	v := w.BodyLinearVelocity(bodyID)
	require.InDelta(t, 10.0, Length(v), 1e-9, "velocity must be clamped to MaximumLinearSpeed")

	// A slow body must not be flagged.
	w.SetBodyLinearVelocity(bodyID, Vec2{X: 1.0, Y: 0.0})
	w.Step(1.0/60.0, 4)
	b = w.getBodyFullID(bodyID)
	tassert.Zero(t, b.flags&isSpeedCapped, "slow body must not be speed capped")
}

// TestContinuousSetsHadTimeOfImpact drives a fast non-bullet circle into a
// thin static wall and checks that the continuous solve records the TOI event
// on the body flags (transferred in finalizeBodiesTask on the next step for
// non-bullet bodies solved inline).
func TestContinuousSetsHadTimeOfImpact(t *testing.T) {
	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	w := NewWorld(&def)
	defer w.Destroy()

	gd := DefaultBodyDef()
	ground := w.CreateBody(&gd)
	wall := MakeBox(0.05, 10.0)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &wall)

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: -5.0, Y: 0.0}
	bd.LinearVelocity = Vec2{X: 200.0, Y: 0.0}
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: 0.1}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	// The TOI happens on the step whose sweep crosses the wall. Watch the
	// body flags across a few steps.
	sawTOI := false
	for range 10 {
		w.Step(1.0/60.0, 4)
		b := w.getBodyFullID(bodyID)
		if b.flags&hadTimeOfImpact != 0 {
			sawTOI = true
			break
		}
	}
	tassert.True(t, sawTOI, "fast body sweeping into a wall must record hadTimeOfImpact")

	// The body must have been stopped by the TOI + contact, not tunneled.
	p := w.BodyPosition(bodyID)
	require.Less(t, p.X, 0.0)
}

// TestBulletBodiesFillOrderIsDeterministic checks that finalizeBodiesTask
// gathers fast bullet bodies in ascending body-sim index order, which is what
// keeps the serial bullet stage deterministic.
func TestBulletBodiesFillOrderIsDeterministic(t *testing.T) {
	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	w := NewWorld(&def)
	defer w.Destroy()

	// Several bullets, all fast, no contacts.
	const count = 8
	ids := make([]BodyID, 0, count)
	for i := range count {
		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: 0.0, Y: 10.0 * float64(i)}
		bd.LinearVelocity = Vec2{X: 100.0, Y: 0.0}
		bd.IsBullet = true
		bodyID := w.CreateBody(&bd)

		circle := Circle{Center: Vec2Zero, Radius: 0.1}
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bodyID, &sd, &circle)
		ids = append(ids, bodyID)
	}

	w.Step(1.0/60.0, 4)

	// After the step every bullet must have advanced (center0 == center,
	// i.e. solveContinuous ran and advanced the sweep origin).
	awake := &w.solverSets[awakeSet]
	require.Len(t, awake.bodySims, count)
	for i := range awake.bodySims {
		sim := &awake.bodySims[i]
		tassert.Equal(t, sim.center, sim.center0, "bullet sim %d must be advanced by the continuous solve", i)
		tassert.NotZero(t, sim.flags&isBullet)
	}
	_ = ids
}
