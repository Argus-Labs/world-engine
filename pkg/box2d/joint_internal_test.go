// Tests for the float64 port of Box2D v3.2.0 src/joint.c (stage E8): joint
// definition constructors, body edge lists, constraint graph coloring, island
// links and solver-set placement. Internal package tests because they inspect
// joint, island, graph and solver-set internals directly.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultJointDefs(t *testing.T) {
	base := defaultJointDef()
	tassert.InDelta(t, 1.0, base.LocalFrameA.Q.C, 0)
	tassert.InDelta(t, 1.0, base.LocalFrameB.Q.C, 0)
	tassert.InDelta(t, 60.0, base.ConstraintHertz, 0)
	tassert.InDelta(t, 2.0, base.ConstraintDampingRatio, 0)
	tassert.InDelta(t, 1.0, base.DrawScale, 0)
	tassert.GreaterOrEqual(t, base.ForceThreshold, maxJointEventThreshold)
	tassert.GreaterOrEqual(t, base.TorqueThreshold, maxJointEventThreshold)

	dd := DefaultDistanceJointDef()
	tassert.True(t, dd.initialized)
	tassert.InDelta(t, 1.0, dd.Length, 0)
	tassert.InDelta(t, Huge, dd.MaxLength, 0)
	tassert.Less(t, dd.LowerSpringForce, 0.0)
	tassert.Greater(t, dd.UpperSpringForce, 0.0)

	md := DefaultMotorJointDef()
	tassert.True(t, md.initialized)

	fd := DefaultFilterJointDef()
	tassert.True(t, fd.initialized)

	pd := DefaultPrismaticJointDef()
	tassert.True(t, pd.initialized)

	rd := DefaultRevoluteJointDef()
	tassert.True(t, rd.initialized)

	wd := DefaultWeldJointDef()
	tassert.True(t, wd.initialized)

	whd := DefaultWheelJointDef()
	tassert.True(t, whd.initialized)
	tassert.True(t, whd.EnableSpring)
	tassert.InDelta(t, 1.0, whd.Hertz, 0)
	tassert.InDelta(t, 0.7, whd.DampingRatio, 0)

	ed := DefaultExplosionDef()
	tassert.Equal(t, DefaultMaskBits, ed.MaskBits)
}

// jointTestBody creates a dynamic circle body for joint bookkeeping tests.
func jointTestBody(t *testing.T, w *World, pos Vec2) BodyID {
	t.Helper()
	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = pos
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: 0.25}
	sd := DefaultShapeDef()
	w.CreateCircleShape(bodyID, &sd, &circle)
	return bodyID
}

func TestJointCreateDestroyBookkeeping(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyIDA := jointTestBody(t, w, Vec2{X: 0.0, Y: 0.0})
	bodyIDB := jointTestBody(t, w, Vec2{X: 1.0, Y: 0.0})

	jd := DefaultDistanceJointDef()
	jd.Base.BodyIDA = bodyIDA
	jd.Base.BodyIDB = bodyIDB
	jd.Base.LocalFrameB.P = Vec2{X: -1.0, Y: 0.0}
	jointID := w.CreateDistanceJoint(&jd)
	require.True(t, w.IsJointValid(jointID))

	j := w.getJointFullID(jointID)
	rawID := j.jointID

	// Awake placement: joint lives in a graph color.
	require.Equal(t, awakeSet, j.setIndex)
	require.True(t, 0 <= j.colorIndex && j.colorIndex < GraphColorCount)
	color := &w.constraintGraph.colors[j.colorIndex]
	require.Equal(t, rawID, color.jointSims[j.localIndex].jointID)

	// Body edge lists.
	bodyA := w.getBodyFullID(bodyIDA)
	bodyB := w.getBodyFullID(bodyIDB)
	require.Equal(t, 1, bodyA.jointCount)
	require.Equal(t, 1, bodyB.jointCount)
	require.Equal(t, rawID<<1, bodyA.headJointKey) // | 0 for edge index 0
	require.Equal(t, (rawID<<1)|1, bodyB.headJointKey)

	// Island link: the two body islands merged and hold the joint.
	require.NotEqual(t, NullIndex, j.islandID)
	require.Equal(t, bodyA.islandID, j.islandID)
	require.Equal(t, bodyB.islandID, j.islandID)
	isl := &w.islands[j.islandID]
	require.Len(t, isl.joints, 1)
	require.Equal(t, rawID, isl.joints[0].jointID)

	generation := j.generation

	// Destroy: everything is unlinked and the id slot is freed.
	w.DestroyJoint(jointID, true)
	require.False(t, w.IsJointValid(jointID))
	require.Equal(t, NullIndex, w.joints[rawID].jointID)
	require.Equal(t, 0, bodyA.jointCount)
	require.Equal(t, 0, bodyB.jointCount)
	require.Equal(t, NullIndex, bodyA.headJointKey)
	require.Equal(t, NullIndex, bodyB.headJointKey)
	require.Empty(t, isl.joints)
	require.Positive(t, isl.constraintRemoveCount)

	// Recreating in the same slot bumps the generation; the stale id stays
	// rejected.
	jointID2 := w.CreateDistanceJoint(&jd)
	j2 := w.getJointFullID(jointID2)
	require.Equal(t, rawID, j2.jointID)
	require.Equal(t, generation+1, j2.generation)
	require.True(t, w.IsJointValid(jointID2))
	require.False(t, w.IsJointValid(jointID))
}

func TestJointStaticAndDisabledPlacement(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	// Two static bodies: the joint goes to the static set and has no island.
	bd := DefaultBodyDef()
	bd.Position = Vec2{X: 0.0, Y: 0.0}
	staticA := w.CreateBody(&bd)
	bd.Position = Vec2{X: 2.0, Y: 0.0}
	staticB := w.CreateBody(&bd)

	fd := DefaultFilterJointDef()
	fd.Base.BodyIDA = staticA
	fd.Base.BodyIDB = staticB
	staticJointID := w.CreateFilterJoint(&fd)

	js := w.getJointFullID(staticJointID)
	require.Equal(t, staticSet, js.setIndex)
	require.Equal(t, NullIndex, js.colorIndex)
	require.Equal(t, NullIndex, js.islandID)

	// A disabled body forces the joint into the disabled set.
	bd = DefaultBodyDef()
	bd.Type = DynamicBody
	bd.IsEnabled = false
	bd.Position = Vec2{X: 4.0, Y: 0.0}
	disabledBody := w.CreateBody(&bd)

	fd2 := DefaultFilterJointDef()
	fd2.Base.BodyIDA = staticA
	fd2.Base.BodyIDB = disabledBody
	disabledJointID := w.CreateFilterJoint(&fd2)

	jd := w.getJointFullID(disabledJointID)
	require.Equal(t, disabledSet, jd.setIndex)
	require.Equal(t, NullIndex, jd.colorIndex)
	require.Equal(t, NullIndex, jd.islandID)
}

func TestJointSleepTransferAndSleepingSetMerge(t *testing.T) {
	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	w := NewWorld(&def)
	defer w.Destroy()

	bodyIDA := jointTestBody(t, w, Vec2{X: 0.0, Y: 0.0})
	bodyIDB := jointTestBody(t, w, Vec2{X: 10.0, Y: 0.0})

	// Let both single-body islands fall asleep in separate sleeping sets.
	for range 60 {
		w.Step(1.0/60.0, 4)
	}

	bodyA := w.getBodyFullID(bodyIDA)
	bodyB := w.getBodyFullID(bodyIDB)
	require.GreaterOrEqual(t, bodyA.setIndex, firstSleepingSet)
	require.GreaterOrEqual(t, bodyB.setIndex, firstSleepingSet)
	require.NotEqual(t, bodyA.setIndex, bodyB.setIndex)

	// Creating a joint between two different sleeping sets merges the sets
	// and leaves everything asleep.
	jd := DefaultDistanceJointDef()
	jd.Base.BodyIDA = bodyIDA
	jd.Base.BodyIDB = bodyIDB
	jd.Length = 10.0
	jd.Base.LocalFrameB.P = Vec2Zero
	jointID := w.CreateDistanceJoint(&jd)

	j := w.getJointFullID(jointID)
	require.GreaterOrEqual(t, j.setIndex, firstSleepingSet)
	require.Equal(t, bodyA.setIndex, bodyB.setIndex)
	require.Equal(t, bodyA.setIndex, j.setIndex)
	require.Equal(t, NullIndex, j.colorIndex)

	// The joint sim lives in the sleeping set's jointSims array.
	set := &w.solverSets[j.setIndex]
	require.Equal(t, j.jointID, set.jointSims[j.localIndex].jointID)

	// Waking through the joint moves the joint back into the constraint
	// graph (addJointToGraph via wakeSolverSet).
	w.WakeJointBodies(jointID)
	require.Equal(t, awakeSet, j.setIndex)
	require.True(t, 0 <= j.colorIndex && j.colorIndex < GraphColorCount)
	require.Equal(t, awakeSet, bodyA.setIndex)
	require.Equal(t, awakeSet, bodyB.setIndex)

	// The joined pair goes back to sleep together and the joint transfers to
	// the new sleeping set (transferJoint path in trySleepIsland).
	for range 60 {
		w.Step(1.0/60.0, 4)
	}
	require.GreaterOrEqual(t, j.setIndex, firstSleepingSet)
	require.Equal(t, bodyA.setIndex, j.setIndex)
	require.Equal(t, bodyB.setIndex, j.setIndex)
	require.Equal(t, NullIndex, j.colorIndex)
}
