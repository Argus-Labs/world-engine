// Behavior tests for the float64 port of the Box2D v3.2.0 weld joint
// (stage E9a, src/weld_joint.c): rigid welds hold the relative transform,
// soft welds (hertz > 0) sag and settle, reaction getters, accessor
// round-trips, destroying a weld mid-simulation and cross-world determinism.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const weldTimeStep = 1.0 / 60.0

// weldTestWorld creates a world with the given gravity and sleeping disabled,
// so a weld under load keeps being solved for the whole run.
func weldTestWorld(gravity box2d.Vec2) *box2d.World {
	def := box2d.DefaultWorldDef()
	def.Gravity = gravity
	def.EnableSleep = false
	return box2d.NewWorld(&def)
}

// weldTestBoxHalfExtent is the half extent of every box in these tests, which
// makes each body weigh exactly 1 kg at density 1.
const weldTestBoxHalfExtent = 0.5

// weldTestBoxBody creates a dynamic 1 kg square body.
func weldTestBoxBody(w *box2d.World, pos box2d.Vec2) box2d.BodyID {
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = pos
	bodyID := w.CreateBody(&bd)

	box := box2d.MakeBox(weldTestBoxHalfExtent, weldTestBoxHalfExtent)
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	w.CreatePolygonShape(bodyID, &sd, &box)
	return bodyID
}

// weldTestRelativeTransform returns the transform of bodyB in the frame of
// bodyA, the quantity a rigid weld must hold constant.
func weldTestRelativeTransform(w *box2d.World, bodyA, bodyB box2d.BodyID) box2d.Transform {
	return box2d.InvMulTransforms(w.BodyTransform(bodyA), w.BodyTransform(bodyB))
}

func TestWeldJointRigidHoldsRelativeTransform(t *testing.T) {
	t.Parallel()

	w := weldTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	bodyA := weldTestBoxBody(w, box2d.Vec2{X: 0.0, Y: 0.0})
	bodyB := weldTestBoxBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

	def := box2d.DefaultWeldJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	def.Base.LocalFrameA.P = box2d.Vec2{X: 0.5, Y: 0.0}
	def.Base.LocalFrameB.P = box2d.Vec2{X: -0.5, Y: 0.0}
	w.CreateWeldJoint(&def)

	initial := weldTestRelativeTransform(w, bodyA, bodyB)

	// Kick body B off center so the weld has to carry a torque as well.
	w.ApplyBodyLinearImpulse(bodyB, box2d.Vec2{X: 0.0, Y: 1.5}, box2d.Vec2{X: 1.5, Y: 0.0}, true)

	maxPositionDrift := 0.0
	maxAngleDrift := 0.0
	for range 300 {
		w.Step(weldTimeStep, 4)

		current := weldTestRelativeTransform(w, bodyA, bodyB)
		maxPositionDrift = math.Max(maxPositionDrift, box2d.Length(box2d.Sub(current.P, initial.P)))
		maxAngleDrift = math.Max(maxAngleDrift, math.Abs(box2d.RelativeAngle(initial.Q, current.Q)))
	}

	t.Logf("rigid weld drift: position %g m, angle %g rad", maxPositionDrift, maxAngleDrift)
	require.Less(t, maxPositionDrift, 1.0e-3, "a rigid weld must hold the relative position")
	require.Less(t, maxAngleDrift, 1.0e-3, "a rigid weld must hold the relative rotation")
}

func TestWeldJointSoftOscillatesThenSettles(t *testing.T) {
	t.Parallel()

	w := weldTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	anchorDef := box2d.DefaultBodyDef()
	anchor := w.CreateBody(&anchorDef)
	body := weldTestBoxBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

	def := box2d.DefaultWeldJointDef()
	def.Base.BodyIDA = anchor
	def.Base.BodyIDB = body
	def.Base.LocalFrameA.P = box2d.Vec2{X: 1.0, Y: 0.0}
	def.LinearHertz = 2.0
	def.LinearDampingRatio = 0.2
	def.AngularHertz = 2.0
	def.AngularDampingRatio = 0.2
	w.CreateWeldJoint(&def)

	const totalSteps = 900
	heights := make([]float64, 0, totalSteps)
	for range totalSteps {
		w.Step(weldTimeStep, 4)
		heights = append(heights, w.BodyPosition(body).Y)
	}

	// Oscillation: the height must cross its final value several times early
	// on.
	settled := heights[len(heights)-1]
	crossings := 0
	for i := 1; i < 300; i++ {
		if (heights[i-1]-settled)*(heights[i]-settled) < 0.0 {
			crossings++
		}
	}

	// Settling: late motion is small and the velocity has decayed.
	lateMin, lateMax := heights[totalSteps-120], heights[totalSteps-120]
	for _, h := range heights[totalSteps-120:] {
		lateMin = math.Min(lateMin, h)
		lateMax = math.Max(lateMax, h)
	}

	t.Logf("soft weld: crossings %d, settled y %g m, late band %g m", crossings, settled, lateMax-lateMin)
	require.GreaterOrEqual(t, crossings, 2, "a lightly damped soft weld should oscillate")
	require.Less(t, lateMax-lateMin, 1.0e-3, "a soft weld should settle")
	require.Less(t, settled, -1.0e-3, "a soft weld should sag under gravity")
	require.Less(t, math.Abs(w.BodyAngularVelocity(body)), 1.0e-2, "the soft weld should stop spinning")
}

func TestWeldJointRigidSagsLessThanSoft(t *testing.T) {
	t.Parallel()

	sag := func(hertz float64) float64 {
		w := weldTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
		defer w.Destroy()

		anchorDef := box2d.DefaultBodyDef()
		anchor := w.CreateBody(&anchorDef)
		body := weldTestBoxBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

		def := box2d.DefaultWeldJointDef()
		def.Base.BodyIDA = anchor
		def.Base.BodyIDB = body
		def.Base.LocalFrameA.P = box2d.Vec2{X: 1.0, Y: 0.0}
		def.LinearHertz = hertz
		def.LinearDampingRatio = 1.0
		def.AngularHertz = hertz
		def.AngularDampingRatio = 1.0
		w.CreateWeldJoint(&def)

		for range 600 {
			w.Step(weldTimeStep, 4)
		}
		return math.Abs(w.BodyPosition(body).Y)
	}

	rigid := sag(0.0)
	soft := sag(1.0)
	t.Logf("rigid sag %g m, soft sag %g m", rigid, soft)

	require.Less(t, rigid, 1.0e-3, "a rigid weld must not sag")
	require.Greater(t, soft, rigid, "a soft weld must sag more than a rigid one")
}

func TestWeldJointReactionGetters(t *testing.T) {
	t.Parallel()

	w := weldTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	anchorDef := box2d.DefaultBodyDef()
	anchor := w.CreateBody(&anchorDef)
	// A 1 kg body hanging one meter out from the anchor: the weld carries its
	// weight and the resulting moment.
	body := weldTestBoxBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

	def := box2d.DefaultWeldJointDef()
	def.Base.BodyIDA = anchor
	def.Base.BodyIDB = body
	def.Base.LocalFrameA.P = box2d.Vec2{X: 1.0, Y: 0.0}
	jointID := w.CreateWeldJoint(&def)

	for range 240 {
		w.Step(weldTimeStep, 4)
	}

	force := w.JointConstraintForce(jointID)
	torque := w.JointConstraintTorque(jointID)
	t.Logf("weld force (%g, %g) N, torque %g N*m", force.X, force.Y, torque)

	require.True(t, box2d.IsValidVec2(force))
	require.True(t, box2d.IsValidFloat(torque))
	require.InDelta(t, 1.0, w.BodyMass(body), 1.0e-9)
	require.InDelta(t, 10.0, box2d.Length(force), 1.0, "a static weld must carry the body weight")
	require.Less(t, math.Abs(torque), 1.0e-6, "an anchor at the center of mass carries no moment")
}

func TestWeldJointAccessorRoundTrip(t *testing.T) {
	t.Parallel()

	w := weldTestWorld(box2d.Vec2Zero)
	defer w.Destroy()

	bodyA := weldTestBoxBody(w, box2d.Vec2Zero)
	bodyB := weldTestBoxBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

	def := box2d.DefaultWeldJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	def.LinearHertz = 3.0
	def.LinearDampingRatio = 0.4
	def.AngularHertz = 5.0
	def.AngularDampingRatio = 0.6
	jointID := w.CreateWeldJoint(&def)

	require.InDelta(t, 3.0, w.WeldJointLinearHertz(jointID), 0.0)
	require.InDelta(t, 0.4, w.WeldJointLinearDampingRatio(jointID), 0.0)
	require.InDelta(t, 5.0, w.WeldJointAngularHertz(jointID), 0.0)
	require.InDelta(t, 0.6, w.WeldJointAngularDampingRatio(jointID), 0.0)

	w.SetWeldJointLinearHertz(jointID, 7.5)
	require.InDelta(t, 7.5, w.WeldJointLinearHertz(jointID), 0.0)

	w.SetWeldJointLinearDampingRatio(jointID, 1.25)
	require.InDelta(t, 1.25, w.WeldJointLinearDampingRatio(jointID), 0.0)

	w.SetWeldJointAngularHertz(jointID, 8.5)
	require.InDelta(t, 8.5, w.WeldJointAngularHertz(jointID), 0.0)

	w.SetWeldJointAngularDampingRatio(jointID, 2.25)
	require.InDelta(t, 2.25, w.WeldJointAngularDampingRatio(jointID), 0.0)

	// Zero hertz means rigid and must round-trip too.
	w.SetWeldJointLinearHertz(jointID, 0.0)
	w.SetWeldJointAngularHertz(jointID, 0.0)
	require.InDelta(t, 0.0, w.WeldJointLinearHertz(jointID), 0.0)
	require.InDelta(t, 0.0, w.WeldJointAngularHertz(jointID), 0.0)
}

func TestWeldJointDestroyMidSimulation(t *testing.T) {
	t.Parallel()

	w := weldTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	anchorDef := box2d.DefaultBodyDef()
	anchor := w.CreateBody(&anchorDef)
	body := weldTestBoxBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

	def := box2d.DefaultWeldJointDef()
	def.Base.BodyIDA = anchor
	def.Base.BodyIDB = body
	def.Base.LocalFrameA.P = box2d.Vec2{X: 1.0, Y: 0.0}
	jointID := w.CreateWeldJoint(&def)

	for range 60 {
		w.Step(weldTimeStep, 4)
	}
	held := w.BodyPosition(body).Y
	require.Less(t, math.Abs(held), 1.0e-3, "the weld must hold the body before it is destroyed")

	w.DestroyJoint(jointID, true)

	for range 60 {
		w.Step(weldTimeStep, 4)
	}
	require.Less(t, w.BodyPosition(body).Y, held-0.4, "the body must fall once the weld is gone")

	// The world stays consistent afterwards.
	for range 60 {
		w.Step(weldTimeStep, 4)
	}
	require.True(t, box2d.IsValidVec2(w.BodyPosition(body)))
}

// buildWeldDeterminismScene builds a short chain of boxes welded to a static
// anchor, with a soft weld at the tip.
func buildWeldDeterminismScene(w *box2d.World) []box2d.BodyID {
	anchorDef := box2d.DefaultBodyDef()
	anchor := w.CreateBody(&anchorDef)

	var bodies []box2d.BodyID
	prev := anchor
	for i := range 4 {
		body := weldTestBoxBody(w, box2d.Vec2{X: float64(i) + 1.0, Y: 0.0})

		def := box2d.DefaultWeldJointDef()
		def.Base.BodyIDA = prev
		def.Base.BodyIDB = body
		if i == 0 {
			def.Base.LocalFrameA.P = box2d.Vec2{X: 1.0, Y: 0.0}
		} else {
			def.Base.LocalFrameA.P = box2d.Vec2{X: 0.5, Y: 0.0}
		}
		def.Base.LocalFrameB.P = box2d.Vec2{X: -0.5, Y: 0.0}
		if i == 3 {
			def.LinearHertz = 3.0
			def.LinearDampingRatio = 0.5
			def.AngularHertz = 3.0
			def.AngularDampingRatio = 0.5
		}
		w.CreateWeldJoint(&def)

		bodies = append(bodies, body)
		prev = body
	}

	return bodies
}

func TestWeldJointDeterminism(t *testing.T) {
	t.Parallel()

	run := func() []uint64 {
		w := weldTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
		defer w.Destroy()

		bodies := buildWeldDeterminismScene(w)
		hashes := make([]uint64, 0, 200)
		for range 200 {
			w.Step(weldTimeStep, 4)
			hashes = append(hashes, hashWorldState(w, bodies))
		}
		return hashes
	}

	first := run()
	second := run()
	require.Equal(t, first, second, "weld joint stepping must be bit-identical across worlds")
}
