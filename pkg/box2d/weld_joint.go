// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/weld_joint.c
// (plus the b2WeldJoint struct from src/joint.h).
//
// Public API mapping: b2WeldJoint_SetLinearHertz →
// (*World).SetWeldJointLinearHertz, b2WeldJoint_GetLinearHertz →
// (*World).WeldJointLinearHertz, and so on (getters are Get-less).
//
// Deviation from upstream: the B2_WELD_BLOCK_SOLVE branch of b2SolveWeldJoint
// is compiled out upstream (the macro is 0 because block solving does not work
// with mixed stiffness values) and is not ported.
//
// TODO(E13b): b2DrawWeldJoint is deferred to the debug draw stage.

package box2d

// weldJoint is the weld joint solver state (upstream b2WeldJoint in joint.h).
// Lives in the jointSim per-type union.
type weldJoint struct {
	linearHertz         float64
	linearDampingRatio  float64
	angularHertz        float64
	angularDampingRatio float64

	linearSpring   softness
	angularSpring  softness
	linearImpulse  Vec2
	angularImpulse float64

	indexA      int
	indexB      int
	frameA      Transform
	frameB      Transform
	deltaCenter Vec2
	axialMass   float64
}

// SetWeldJointLinearHertz sets the weld joint linear stiffness in Hertz. Zero
// means rigid (upstream b2WeldJoint_SetLinearHertz).
func (w *World) SetWeldJointLinearHertz(jointID JointID, hertz float64) {
	assert(IsValidFloat(hertz) && hertz >= 0.0)
	j := w.getJointSimCheckType(jointID, WeldJoint)
	j.weldJoint.linearHertz = hertz
}

// WeldJointLinearHertz returns the weld joint linear stiffness in Hertz
// (upstream b2WeldJoint_GetLinearHertz).
func (w *World) WeldJointLinearHertz(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WeldJoint)
	return j.weldJoint.linearHertz
}

// SetWeldJointLinearDampingRatio sets the weld joint linear damping ratio,
// non-dimensional (upstream b2WeldJoint_SetLinearDampingRatio).
func (w *World) SetWeldJointLinearDampingRatio(jointID JointID, dampingRatio float64) {
	assert(IsValidFloat(dampingRatio) && dampingRatio >= 0.0)
	j := w.getJointSimCheckType(jointID, WeldJoint)
	j.weldJoint.linearDampingRatio = dampingRatio
}

// WeldJointLinearDampingRatio returns the weld joint linear damping ratio
// (upstream b2WeldJoint_GetLinearDampingRatio).
func (w *World) WeldJointLinearDampingRatio(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WeldJoint)
	return j.weldJoint.linearDampingRatio
}

// SetWeldJointAngularHertz sets the weld joint angular stiffness in Hertz.
// Zero means rigid (upstream b2WeldJoint_SetAngularHertz).
func (w *World) SetWeldJointAngularHertz(jointID JointID, hertz float64) {
	assert(IsValidFloat(hertz) && hertz >= 0.0)
	j := w.getJointSimCheckType(jointID, WeldJoint)
	j.weldJoint.angularHertz = hertz
}

// WeldJointAngularHertz returns the weld joint angular stiffness in Hertz
// (upstream b2WeldJoint_GetAngularHertz).
func (w *World) WeldJointAngularHertz(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WeldJoint)
	return j.weldJoint.angularHertz
}

// SetWeldJointAngularDampingRatio sets the weld joint angular damping ratio,
// non-dimensional (upstream b2WeldJoint_SetAngularDampingRatio).
func (w *World) SetWeldJointAngularDampingRatio(jointID JointID, dampingRatio float64) {
	assert(IsValidFloat(dampingRatio) && dampingRatio >= 0.0)
	j := w.getJointSimCheckType(jointID, WeldJoint)
	j.weldJoint.angularDampingRatio = dampingRatio
}

// WeldJointAngularDampingRatio returns the weld joint angular damping ratio
// (upstream b2WeldJoint_GetAngularDampingRatio).
func (w *World) WeldJointAngularDampingRatio(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WeldJoint)
	return j.weldJoint.angularDampingRatio
}

// getWeldJointForce returns the weld joint constraint force
// (upstream b2GetWeldJointForce).
func getWeldJointForce(w *World, base *jointSim) Vec2 {
	force := MulSV(w.invH, base.weldJoint.linearImpulse)
	return force
}

// getWeldJointTorque returns the weld joint constraint torque
// (upstream b2GetWeldJointTorque).
func getWeldJointTorque(w *World, base *jointSim) float64 {
	return w.invH * base.weldJoint.angularImpulse
}

// Point-to-point constraint
// C = p2 - p1
// Cdot = v2 - v1
//      = v2 + cross(w2, r2) - v1 - cross(w1, r1)
// J = [-E -r1_skew E r2_skew ]
// Identity used:
// w k % (rx i + ry j) = w * (-ry i + rx j)

// Angle constraint
// C = angle2 - angle1 - referenceAngle
// Cdot = w2 - w1
// J = [0 0 -1 0 0 1]
// K = invI1 + invI2

// prepareWeldJoint mirrors b2PrepareWeldJoint.
func prepareWeldJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == WeldJoint)

	// chase body id to the solver set where the body lives
	idA := base.bodyIDA
	idB := base.bodyIDB

	w := ctx.world

	bodyA := &w.bodies[idA]
	bodyB := &w.bodies[idB]

	assert(bodyA.setIndex == awakeSet || bodyB.setIndex == awakeSet)
	setA := &w.solverSets[bodyA.setIndex]
	setB := &w.solverSets[bodyB.setIndex]

	localIndexA := bodyA.localIndex
	localIndexB := bodyB.localIndex

	bodySimA := &setA.bodySims[localIndexA]
	bodySimB := &setB.bodySims[localIndexB]

	mA := bodySimA.invMass
	iA := bodySimA.invInertia
	mB := bodySimB.invMass
	iB := bodySimB.invInertia

	base.invMassA = mA
	base.invMassB = mB
	base.invIA = iA
	base.invIB = iB

	j := &base.weldJoint

	j.indexA = NullIndex
	if bodyA.setIndex == awakeSet {
		j.indexA = localIndexA
	}
	j.indexB = NullIndex
	if bodyB.setIndex == awakeSet {
		j.indexB = localIndexB
	}

	// Compute joint anchor frames with world space rotation, relative to
	// center of mass.
	j.frameA.Q = MulRot(bodySimA.transform.Q, base.localFrameA.Q)
	j.frameA.P = RotateVector(bodySimA.transform.Q, Sub(base.localFrameA.P, bodySimA.localCenter))
	j.frameB.Q = MulRot(bodySimB.transform.Q, base.localFrameB.Q)
	j.frameB.P = RotateVector(bodySimB.transform.Q, Sub(base.localFrameB.P, bodySimB.localCenter))

	// Compute the initial center delta. Incremental position updates are
	// relative to this.
	j.deltaCenter = Sub(bodySimB.center, bodySimA.center)

	ka := iA + iB
	j.axialMass = 0.0
	if ka > 0.0 {
		j.axialMass = 1.0 / ka
	}

	if j.linearHertz == 0.0 {
		j.linearSpring = base.constraintSoftness
	} else {
		j.linearSpring = makeSoft(j.linearHertz, j.linearDampingRatio, ctx.h)
	}

	if j.angularHertz == 0.0 {
		j.angularSpring = base.constraintSoftness
	} else {
		j.angularSpring = makeSoft(j.angularHertz, j.angularDampingRatio, ctx.h)
	}

	if !ctx.enableWarmStarting {
		j.linearImpulse = Vec2Zero
		j.angularImpulse = 0.0
	}
}

// warmStartWeldJoint mirrors b2WarmStartWeldJoint.
func warmStartWeldJoint(base *jointSim, ctx *stepContext) {
	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.weldJoint

	stateA := &dummyState
	if j.indexA != NullIndex {
		stateA = &ctx.states[j.indexA]
	}
	stateB := &dummyState
	if j.indexB != NullIndex {
		stateB = &ctx.states[j.indexB]
	}

	rA := RotateVector(stateA.deltaRotation, j.frameA.P)
	rB := RotateVector(stateB.deltaRotation, j.frameB.P)

	if stateA.flags&dynamicFlag != 0 {
		stateA.linearVelocity = MulSub(stateA.linearVelocity, mA, j.linearImpulse)
		// wA -= iA * (cross(rA, linearImpulse) + angularImpulse)
		stateA.angularVelocity -= float64(iA * (Cross(rA, j.linearImpulse) + j.angularImpulse))
	}

	if stateB.flags&dynamicFlag != 0 {
		stateB.linearVelocity = MulAdd(stateB.linearVelocity, mB, j.linearImpulse)
		// wB += iB * (cross(rB, linearImpulse) + angularImpulse)
		stateB.angularVelocity += float64(iB * (Cross(rB, j.linearImpulse) + j.angularImpulse))
	}
}

// solveWeldJoint mirrors b2SolveWeldJoint.
func solveWeldJoint(base *jointSim, ctx *stepContext, useBias bool) {
	assert(base.jointType == WeldJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.weldJoint

	stateA := &dummyState
	if j.indexA != NullIndex {
		stateA = &ctx.states[j.indexA]
	}
	stateB := &dummyState
	if j.indexB != NullIndex {
		stateB = &ctx.states[j.indexB]
	}

	vA := stateA.linearVelocity
	wA := stateA.angularVelocity
	vB := stateB.linearVelocity
	wB := stateB.angularVelocity

	// angular constraint
	{
		qA := MulRot(stateA.deltaRotation, j.frameA.Q)
		qB := MulRot(stateB.deltaRotation, j.frameB.Q)
		relQ := InvMulRot(qA, qB)
		jointAngle := RotGetAngle(relQ)

		bias := 0.0
		massScale := 1.0
		impulseScale := 0.0
		if useBias || j.angularHertz > 0.0 {
			c := jointAngle
			bias = j.angularSpring.biasRate * c
			massScale = j.angularSpring.massScale
			impulseScale = j.angularSpring.impulseScale
		}

		cDot := wB - wA
		// impulse = -massScale * axialMass * (Cdot + bias) - impulseScale * angularImpulse
		impulse := cross2(-massScale*j.axialMass, cDot+bias, impulseScale, j.angularImpulse)
		j.angularImpulse += impulse

		// wA -= iA * impulse
		wA -= float64(iA * impulse)
		// wB += iB * impulse
		wB += float64(iB * impulse)
	}

	// linear constraint
	{
		rA := RotateVector(stateA.deltaRotation, j.frameA.P)
		rB := RotateVector(stateB.deltaRotation, j.frameB.P)

		bias := Vec2Zero
		massScale := 1.0
		impulseScale := 0.0
		if useBias || j.linearHertz > 0.0 {
			dcA := stateA.deltaPosition
			dcB := stateB.deltaPosition
			c := Add(Add(Sub(dcB, dcA), Sub(rB, rA)), j.deltaCenter)

			bias = MulSV(j.linearSpring.biasRate, c)
			massScale = j.linearSpring.massScale
			impulseScale = j.linearSpring.impulseScale
		}

		cDot := Sub(Add(vB, CrossSV(wB, rB)), Add(vA, CrossSV(wA, rA)))

		var k Mat22
		// K.cx.x = mA + mB + rA.y * rA.y * iA + rB.y * rB.y * iB
		k.CX.X = mA + mB + float64(rA.Y*rA.Y*iA) + float64(rB.Y*rB.Y*iB)
		// K.cy.x = -rA.y * rA.x * iA - rB.y * rB.x * iB
		k.CY.X = float64(-rA.Y*rA.X*iA) - float64(rB.Y*rB.X*iB)
		k.CX.Y = k.CY.X
		// K.cy.y = mA + mB + rA.x * rA.x * iA + rB.x * rB.x * iB
		k.CY.Y = mA + mB + float64(rA.X*rA.X*iA) + float64(rB.X*rB.X*iB)
		b := Solve22(k, Add(cDot, bias))

		var impulse Vec2
		// impulse.x = -massScale * b.x - impulseScale * linearImpulse.x
		impulse.X = cross2(-massScale, b.X, impulseScale, j.linearImpulse.X)
		// impulse.y = -massScale * b.y - impulseScale * linearImpulse.y
		impulse.Y = cross2(-massScale, b.Y, impulseScale, j.linearImpulse.Y)

		j.linearImpulse = Add(j.linearImpulse, impulse)

		vA = MulSub(vA, mA, impulse)
		// wA -= iA * cross(rA, impulse)
		wA -= float64(iA * Cross(rA, impulse))
		vB = MulAdd(vB, mB, impulse)
		// wB += iB * cross(rB, impulse)
		wB += float64(iB * Cross(rB, impulse))
	}

	assert(IsValidVec2(vA))
	assert(IsValidFloat(wA))
	assert(IsValidVec2(vB))
	assert(IsValidFloat(wB))

	if stateA.flags&dynamicFlag != 0 {
		stateA.linearVelocity = vA
		stateA.angularVelocity = wA
	}

	if stateB.flags&dynamicFlag != 0 {
		stateB.linearVelocity = vB
		stateB.angularVelocity = wB
	}
}
