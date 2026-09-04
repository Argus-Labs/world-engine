// Coverage for the LIGHT per-color solver stages: prepare joints, prepare
// contacts, warm start, apply restitution and store impulses. They dispatch at
// solverLightColorGrain (1024) rather than solverColorGrain (32), so a color
// needs 2*solverLightColorGrain items before forRangeWorkers engages a second
// worker — an order of magnitude more than any other test scene reaches. Every
// other suite therefore runs these five stages inline on the dispatching
// goroutine, which leaves their parallel path untested (including under -race).
//
// The scene below exists solely to cross that threshold. Its construction
// leans on the graph-coloring rule in addContactToGraph/addJointToGraph: a
// constraint between a static and a dynamic body claims only the DYNAMIC
// body's bit in the color's body set, so any number of constraints sharing one
// static body still pack into a single color as long as their dynamic ends are
// distinct. Thousands of separate boxes resting on one ground, plus thousands
// of separate pendulum bobs on static anchors, therefore all land in the same
// color — which is what the light stages measure.
//
// This file is an internal test so the premise can be asserted against the
// constraint graph directly (Counters().ColorCounts sums joints and contacts,
// but prepare-joints reads only jointSims and restitution/store-impulses read
// only contactSims, so the sum is not enough). It also exports the builder and
// the grain to the external golden test package, which runs the scene through
// the serial-vs-parallel byte-identity check
// (TestWorkerStressLightStageColorMatchesSerial in golden_workers_test.go).

package box2d

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// SolverLightColorGrainForTest exposes solverLightColorGrain to the external
// box2d_test package, which cannot see unexported identifiers.
const SolverLightColorGrainForTest = solverLightColorGrain

const (
	// lightStageColorItems is the number of constraints of EACH kind (ground
	// contacts and pendulum joints) the scene puts into one color. It must
	// exceed 2*solverLightColorGrain on its own, not summed, because the
	// prepare-joints stage sees only the joints and the prepare-contacts,
	// restitution and store-impulses stages see only the contacts. The margin
	// over 2048 absorbs a box or two that has not settled into contact yet.
	lightStageColorItems = 2200

	// lightStageSpacing keeps neighbours clear of each other: boxes are 1 wide
	// and bobs orbit at radius 1 with a 0.5 diameter, so nothing in the scene
	// touches anything but the ground.
	lightStageSpacing = 1.5

	// lightStageHalfSpan centres the row on the origin.
	lightStageHalfSpan = 0.5 * lightStageSpacing * lightStageColorItems
)

// BuildLightStageColorScene builds the single-color scene described in the file
// header and returns every body it creates (statics included) so callers can
// hash the whole scene.
//
// Float discipline matches the other scene builders: every product is wrapped
// in float64() before it reaches a +/-, so the layout arithmetic cannot fuse
// into an FMA and diverge across architectures (see nofma_test.go).
func BuildLightStageColorScene(w *World) []BodyID {
	bodies := make([]BodyID, 0, 1+3*lightStageColorItems)

	// One wide static ground. Its top edge is y = 0.
	gd := DefaultBodyDef()
	gd.Position = Vec2{X: 0.0, Y: -1.0}
	ground := w.CreateBody(&gd)
	groundBox := MakeBox(lightStageHalfSpan+10.0, 1.0)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)
	bodies = append(bodies, ground)

	// Boxes resting on the ground, one static-vs-dynamic contact each. They
	// start slightly overlapped so the contacts are touching from the first
	// step instead of after a settling delay.
	for i := range lightStageColorItems {
		fi := float64(i)
		x := float64(lightStageSpacing*fi) - lightStageHalfSpan

		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: x, Y: 0.49}
		body := w.CreateBody(&bd)

		box := MakeBox(0.5, 0.5)
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreatePolygonShape(body, &sd, &box)
		bodies = append(bodies, body)
	}

	// Pendulums, one static-vs-dynamic revolute joint each. The anchors carry
	// no shape, so they cost a body slot and nothing in the broad phase.
	for i := range lightStageColorItems {
		fi := float64(i)
		ax := float64(lightStageSpacing*fi) - lightStageHalfSpan

		ad := DefaultBodyDef()
		ad.Position = Vec2{X: ax, Y: 60.0}
		anchor := w.CreateBody(&ad)
		bodies = append(bodies, anchor)

		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: ax + 1.0, Y: 60.0}
		bob := w.CreateBody(&bd)
		circle := Circle{Center: Vec2Zero, Radius: 0.25}
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bob, &sd, &circle)
		bodies = append(bodies, bob)

		jd := DefaultRevoluteJointDef()
		jd.Base.BodyIDA = anchor
		jd.Base.BodyIDB = bob
		jd.Base.LocalFrameB.P = Vec2{X: 1.0, Y: 0.0}
		// Low threshold so the joint-event bit set path runs too.
		jd.Base.ForceThreshold = 1.0
		w.CreateRevoluteJoint(&jd)
	}

	return bodies
}

// TestLightStageColorSceneEngagesMultipleWorkers pins the PREMISE of
// TestWorkerStressLightStageColorMatchesSerial: that its scene really does put
// enough joints and enough contacts into a single color for the five light
// stages to dispatch to the pool. Without this, a future retune of
// solverLightColorGrain (or a scene edit) could silently drop that suite back
// to the inline path and the -race run would still pass while proving nothing.
func TestLightStageColorSceneEngagesMultipleWorkers(t *testing.T) {
	t.Parallel()

	def := DefaultWorldDef()
	def.EnableSleep = false
	w := NewWorld(&def)
	defer w.Destroy()

	BuildLightStageColorScene(w)

	// A few steps so the resting boxes begin touching and their contacts enter
	// the constraint graph.
	for range 5 {
		w.Step(1.0/60.0, 4)
	}

	// Static-vs-dynamic constraints build colors from the end (see
	// addContactToGraph), so both families land in the highest static color.
	color := &w.constraintGraph.colors[overflowIndex-1]
	jointCount := len(color.jointSims)
	contactCount := len(color.contactSims)

	const need = 2 * solverLightColorGrain
	require.GreaterOrEqualf(t, jointCount, need,
		"prepare-joints dispatches on the per-color joint count alone; %d joints in the largest color "+
			"is below the %d needed to engage a second worker at solverLightColorGrain=%d",
		jointCount, need, solverLightColorGrain)
	require.GreaterOrEqualf(t, contactCount, need,
		"prepare-contacts, restitution and store-impulses dispatch on the per-color contact count alone; "+
			"%d contacts in the largest color is below the %d needed to engage a second worker",
		contactCount, need)

	// Warm start dispatches on joints+contacts together, so it fans wider.
	const workerCount = 8
	require.GreaterOrEqual(t, forRangeWorkers(jointCount+contactCount, solverLightColorGrain, workerCount), 4,
		"warm start should reach at least 4 workers on this scene")
}
