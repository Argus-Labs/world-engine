// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/motor_joint.c
// (plus the b2MotorJoint struct from src/joint.h).
//
// Public API mapping: b2MotorJoint_SetLinearVelocity →
// (*World).SetMotorJointLinearVelocity, b2MotorJoint_GetLinearVelocity →
// (*World).MotorJointLinearVelocity, and so on (getters are Get-less).
//
// TODO(E13b): b2DrawJoint's motor case is deferred to the debug draw stage.

package box2d

// motorJoint is the motor joint solver state (upstream b2MotorJoint in
// joint.h). Lives in the jointSim per-type union.
type motorJoint struct {
	linearVelocity      Vec2
	maxVelocityForce    float64
	angularVelocity     float64
	maxVelocityTorque   float64
	linearHertz         float64
	linearDampingRatio  float64
	maxSpringForce      float64
	angularHertz        float64
	angularDampingRatio float64
	maxSpringTorque     float64

	linearVelocityImpulse  Vec2
	angularVelocityImpulse float64
	linearSpringImpulse    Vec2
	angularSpringImpulse   float64

	linearSpring  softness
	angularSpring softness

	indexA      int
	indexB      int
	frameA      Transform
	frameB      Transform
	deltaCenter Vec2
	linearMass  Mat22
	angularMass float64
}

// SetMotorJointLinearVelocity sets the desired relative linear velocity in
// meters per second (upstream b2MotorJoint_SetLinearVelocity).
func (w *World) SetMotorJointLinearVelocity(jointID JointID, velocity Vec2) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.linearVelocity = velocity
}

// MotorJointLinearVelocity returns the desired relative linear velocity in
// meters per second (upstream b2MotorJoint_GetLinearVelocity).
func (w *World) MotorJointLinearVelocity(jointID JointID) Vec2 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.linearVelocity
}

// SetMotorJointAngularVelocity sets the desired relative angular velocity in
// radians per second (upstream b2MotorJoint_SetAngularVelocity).
func (w *World) SetMotorJointAngularVelocity(jointID JointID, velocity float64) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.angularVelocity = velocity
}

// MotorJointAngularVelocity returns the desired relative angular velocity in
// radians per second (upstream b2MotorJoint_GetAngularVelocity).
func (w *World) MotorJointAngularVelocity(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.angularVelocity
}

// SetMotorJointMaxVelocityTorque sets the maximum torque the velocity motor
// may apply, usually in Newton * meters
// (upstream b2MotorJoint_SetMaxVelocityTorque).
func (w *World) SetMotorJointMaxVelocityTorque(jointID JointID, maxTorque float64) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.maxVelocityTorque = maxTorque
}

// MotorJointMaxVelocityTorque returns the maximum velocity motor torque
// (upstream b2MotorJoint_GetMaxVelocityTorque).
func (w *World) MotorJointMaxVelocityTorque(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.maxVelocityTorque
}

// SetMotorJointMaxVelocityForce sets the maximum force the velocity motor may
// apply, usually in Newtons (upstream b2MotorJoint_SetMaxVelocityForce).
func (w *World) SetMotorJointMaxVelocityForce(jointID JointID, maxForce float64) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.maxVelocityForce = maxForce
}

// MotorJointMaxVelocityForce returns the maximum velocity motor force
// (upstream b2MotorJoint_GetMaxVelocityForce).
func (w *World) MotorJointMaxVelocityForce(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.maxVelocityForce
}

// SetMotorJointLinearHertz sets the linear spring stiffness in Hertz used for
// position control (upstream b2MotorJoint_SetLinearHertz).
func (w *World) SetMotorJointLinearHertz(jointID JointID, hertz float64) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.linearHertz = hertz
}

// MotorJointLinearHertz returns the linear spring stiffness in Hertz
// (upstream b2MotorJoint_GetLinearHertz).
func (w *World) MotorJointLinearHertz(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.linearHertz
}

// SetMotorJointLinearDampingRatio sets the linear spring damping ratio,
// non-dimensional (upstream b2MotorJoint_SetLinearDampingRatio).
func (w *World) SetMotorJointLinearDampingRatio(jointID JointID, damping float64) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.linearDampingRatio = damping
}

// MotorJointLinearDampingRatio returns the linear spring damping ratio
// (upstream b2MotorJoint_GetLinearDampingRatio).
func (w *World) MotorJointLinearDampingRatio(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.linearDampingRatio
}

// SetMotorJointAngularHertz sets the angular spring stiffness in Hertz used
// for position control (upstream b2MotorJoint_SetAngularHertz).
func (w *World) SetMotorJointAngularHertz(jointID JointID, hertz float64) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.angularHertz = hertz
}

// MotorJointAngularHertz returns the angular spring stiffness in Hertz
// (upstream b2MotorJoint_GetAngularHertz).
func (w *World) MotorJointAngularHertz(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.angularHertz
}

// SetMotorJointAngularDampingRatio sets the angular spring damping ratio,
// non-dimensional (upstream b2MotorJoint_SetAngularDampingRatio).
func (w *World) SetMotorJointAngularDampingRatio(jointID JointID, damping float64) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.angularDampingRatio = damping
}

// MotorJointAngularDampingRatio returns the angular spring damping ratio
// (upstream b2MotorJoint_GetAngularDampingRatio).
func (w *World) MotorJointAngularDampingRatio(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.angularDampingRatio
}

// SetMotorJointMaxSpringForce sets the maximum spring force in Newtons
// (upstream b2MotorJoint_SetMaxSpringForce).
func (w *World) SetMotorJointMaxSpringForce(jointID JointID, maxForce float64) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.maxSpringForce = maxFloat(0.0, maxForce)
}

// MotorJointMaxSpringForce returns the maximum spring force in Newtons
// (upstream b2MotorJoint_GetMaxSpringForce).
func (w *World) MotorJointMaxSpringForce(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.maxSpringForce
}

// SetMotorJointMaxSpringTorque sets the maximum spring torque in
// Newton * meters (upstream b2MotorJoint_SetMaxSpringTorque).
func (w *World) SetMotorJointMaxSpringTorque(jointID JointID, maxTorque float64) {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	j.motorJoint.maxSpringTorque = maxFloat(0.0, maxTorque)
}

// MotorJointMaxSpringTorque returns the maximum spring torque in
// Newton * meters (upstream b2MotorJoint_GetMaxSpringTorque).
func (w *World) MotorJointMaxSpringTorque(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, MotorJoint)
	return j.motorJoint.maxSpringTorque
}

// getMotorJointForce returns the motor joint constraint force
// (upstream b2GetMotorJointForce).
func getMotorJointForce(w *World, base *jointSim) Vec2 {
	force := MulSV(w.invH, Add(base.motorJoint.linearVelocityImpulse, base.motorJoint.linearSpringImpulse))
	return force
}

// getMotorJointTorque returns the motor joint constraint torque
// (upstream b2GetMotorJointTorque).
func getMotorJointTorque(w *World, base *jointSim) float64 {
	return w.invH * (base.motorJoint.angularVelocityImpulse + base.motorJoint.angularSpringImpulse)
}

// Point-to-point constraint
// C = p2 - p1
// Cdot = v2 - v1
//      = v2 + cross(w2, r2) - v1 - cross(w1, r1)
// J = [-I -r1_skew I r2_skew ]
// Identity used:
// w k % (rx i + ry j) = w * (-ry i + rx j)

// Angle constraint
// C = angle2 - angle1 - referenceAngle
// Cdot = w2 - w1
// J = [0 0 -1 0 0 1]
// K = invI1 + invI2

// prepareMotorJoint mirrors b2PrepareMotorJoint.
func prepareMotorJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == MotorJoint)

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

	j := &base.motorJoint

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

	rA := j.frameA.P
	rB := j.frameB.P

	j.linearSpring = makeSoft(j.linearHertz, j.linearDampingRatio, ctx.h)
	j.angularSpring = makeSoft(j.angularHertz, j.angularDampingRatio, ctx.h)

	var kl Mat22
	// kl.cx.x = mA + mB + rA.y * rA.y * iA + rB.y * rB.y * iB
	kl.CX.X = mA + mB + float64(rA.Y*rA.Y*iA) + float64(rB.Y*rB.Y*iB)
	// kl.cx.y = -rA.y * rA.x * iA - rB.y * rB.x * iB
	kl.CX.Y = float64(-rA.Y*rA.X*iA) - float64(rB.Y*rB.X*iB)
	kl.CY.X = kl.CX.Y
	// kl.cy.y = mA + mB + rA.x * rA.x * iA + rB.x * rB.x * iB
	kl.CY.Y = mA + mB + float64(rA.X*rA.X*iA) + float64(rB.X*rB.X*iB)
	j.linearMass = GetInverse22(kl)

	ka := iA + iB
	j.angularMass = 0.0
	if ka > 0.0 {
		j.angularMass = 1.0 / ka
	}

	if !ctx.enableWarmStarting {
		j.linearVelocityImpulse = Vec2Zero
		j.angularVelocityImpulse = 0.0
		j.linearSpringImpulse = Vec2Zero
		j.angularSpringImpulse = 0.0
	}
}

// warmStartMotorJoint mirrors b2WarmStartMotorJoint.
func warmStartMotorJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == MotorJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	j := &base.motorJoint

	// dummy state for static bodies
	dummyState := identityBodyState

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

	linearImpulse := Add(j.linearVelocityImpulse, j.linearSpringImpulse)
	angularImpulse := j.angularVelocityImpulse + j.angularSpringImpulse

	if stateA.flags&dynamicFlag != 0 {
		stateA.linearVelocity = MulSub(stateA.linearVelocity, mA, linearImpulse)
		// wA -= iA * (cross(rA, linearImpulse) + angularImpulse)
		stateA.angularVelocity -= float64(iA * (Cross(rA, linearImpulse) + angularImpulse))
	}

	if stateB.flags&dynamicFlag != 0 {
		stateB.linearVelocity = MulAdd(stateB.linearVelocity, mB, linearImpulse)
		// wB += iB * (cross(rB, linearImpulse) + angularImpulse)
		stateB.angularVelocity += float64(iB * (Cross(rB, linearImpulse) + angularImpulse))
	}
}

// solveMotorJoint mirrors b2SolveMotorJoint. Note: upstream has no useBias
// parameter for the motor joint.
func solveMotorJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == MotorJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.motorJoint

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

	// angular spring
	if j.maxSpringTorque > 0.0 && j.angularHertz > 0.0 {
		qA := MulRot(stateA.deltaRotation, j.frameA.Q)
		qB := MulRot(stateB.deltaRotation, j.frameB.Q)
		relQ := InvMulRot(qA, qB)

		c := RotGetAngle(relQ)
		bias := float64(j.angularSpring.biasRate * c)
		massScale := j.angularSpring.massScale
		impulseScale := j.angularSpring.impulseScale

		cDot := wB - wA

		maxImpulse := ctx.h * j.maxSpringTorque
		oldImpulse := j.angularSpringImpulse
		// impulse = -massScale * angularMass * (Cdot + bias) - impulseScale * oldImpulse
		impulse := cross2(-massScale*j.angularMass, cDot+bias, impulseScale, oldImpulse)
		j.angularSpringImpulse = clampFloat(oldImpulse+impulse, -maxImpulse, maxImpulse)
		impulse = j.angularSpringImpulse - oldImpulse

		// wA -= iA * impulse
		wA -= float64(iA * impulse)
		// wB += iB * impulse
		wB += float64(iB * impulse)
	}

	// angular velocity
	if j.maxVelocityTorque > 0.0 {
		cDot := wB - wA - j.angularVelocity
		impulse := float64(-j.angularMass * cDot)

		maxImpulse := ctx.h * j.maxVelocityTorque
		oldImpulse := j.angularVelocityImpulse
		j.angularVelocityImpulse = clampFloat(oldImpulse+impulse, -maxImpulse, maxImpulse)
		impulse = j.angularVelocityImpulse - oldImpulse

		wA -= float64(iA * impulse)
		wB += float64(iB * impulse)
	}

	rA := RotateVector(stateA.deltaRotation, j.frameA.P)
	rB := RotateVector(stateB.deltaRotation, j.frameB.P)

	// linear spring
	if j.maxSpringForce > 0.0 && j.linearHertz > 0.0 {
		dcA := stateA.deltaPosition
		dcB := stateB.deltaPosition
		c := Add(Add(Sub(dcB, dcA), Sub(rB, rA)), j.deltaCenter)

		bias := MulSV(j.linearSpring.biasRate, c)
		massScale := j.linearSpring.massScale
		impulseScale := j.linearSpring.impulseScale

		cDot := Sub(Add(vB, CrossSV(wB, rB)), Add(vA, CrossSV(wA, rA)))
		cDot = Add(cDot, bias)

		// Updating the effective mass here may be overkill
		var kl Mat22
		// kl.cx.x = mA + mB + rA.y * rA.y * iA + rB.y * rB.y * iB
		kl.CX.X = mA + mB + float64(rA.Y*rA.Y*iA) + float64(rB.Y*rB.Y*iB)
		// kl.cx.y = -rA.y * rA.x * iA - rB.y * rB.x * iB
		kl.CX.Y = float64(-rA.Y*rA.X*iA) - float64(rB.Y*rB.X*iB)
		kl.CY.X = kl.CX.Y
		// kl.cy.y = mA + mB + rA.x * rA.x * iA + rB.x * rB.x * iB
		kl.CY.Y = mA + mB + float64(rA.X*rA.X*iA) + float64(rB.X*rB.X*iB)
		j.linearMass = GetInverse22(kl)

		b := MulMV(j.linearMass, cDot)

		oldImpulse := j.linearSpringImpulse
		var impulse Vec2
		// impulse.x = -massScale * b.x - impulseScale * oldImpulse.x
		impulse.X = cross2(-massScale, b.X, impulseScale, oldImpulse.X)
		// impulse.y = -massScale * b.y - impulseScale * oldImpulse.y
		impulse.Y = cross2(-massScale, b.Y, impulseScale, oldImpulse.Y)

		maxImpulse := ctx.h * j.maxSpringForce
		j.linearSpringImpulse = Add(j.linearSpringImpulse, impulse)

		if LengthSquared(j.linearSpringImpulse) > maxImpulse*maxImpulse {
			j.linearSpringImpulse = Normalize(j.linearSpringImpulse)
			j.linearSpringImpulse.X *= maxImpulse
			j.linearSpringImpulse.Y *= maxImpulse
		}

		impulse = Sub(j.linearSpringImpulse, oldImpulse)

		vA = MulSub(vA, mA, impulse)
		// wA -= iA * cross(rA, impulse)
		wA -= float64(iA * Cross(rA, impulse))
		vB = MulAdd(vB, mB, impulse)
		// wB += iB * cross(rB, impulse)
		wB += float64(iB * Cross(rB, impulse))
	}

	// linear velocity
	if j.maxVelocityForce > 0.0 {
		cDot := Sub(Add(vB, CrossSV(wB, rB)), Add(vA, CrossSV(wA, rA)))
		cDot = Sub(cDot, j.linearVelocity)
		b := MulMV(j.linearMass, cDot)
		impulse := Vec2{X: -b.X, Y: -b.Y}

		oldImpulse := j.linearVelocityImpulse
		maxImpulse := ctx.h * j.maxVelocityForce
		j.linearVelocityImpulse = Add(j.linearVelocityImpulse, impulse)

		if LengthSquared(j.linearVelocityImpulse) > maxImpulse*maxImpulse {
			j.linearVelocityImpulse = Normalize(j.linearVelocityImpulse)
			j.linearVelocityImpulse.X *= maxImpulse
			j.linearVelocityImpulse.Y *= maxImpulse
		}

		impulse = Sub(j.linearVelocityImpulse, oldImpulse)

		vA = MulSub(vA, mA, impulse)
		wA -= float64(iA * Cross(rA, impulse))
		vB = MulAdd(vB, mB, impulse)
		wB += float64(iB * Cross(rB, impulse))
	}

	if stateA.flags&dynamicFlag != 0 {
		stateA.linearVelocity = vA
		stateA.angularVelocity = wA
	}

	if stateB.flags&dynamicFlag != 0 {
		stateB.linearVelocity = vB
		stateB.angularVelocity = wB
	}
}
