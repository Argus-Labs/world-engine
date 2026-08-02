// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/revolute_joint.c
// (plus the b2RevoluteJoint struct from src/joint.h).
//
// Public API mapping: b2RevoluteJoint_EnableSpring →
// (*World).EnableRevoluteJointSpring, b2RevoluteJoint_GetAngle →
// (*World).RevoluteJointAngle, and so on (getters are Get-less).
//

package box2d

import "strconv"

// revoluteJoint is the revolute joint solver state (upstream b2RevoluteJoint
// in joint.h). Lives in the jointSim per-type union.
type revoluteJoint struct {
	linearImpulse  Vec2
	springImpulse  float64
	motorImpulse   float64
	lowerImpulse   float64
	upperImpulse   float64
	hertz          float64
	dampingRatio   float64
	targetAngle    float64
	maxMotorTorque float64
	motorSpeed     float64
	lowerAngle     float64
	upperAngle     float64

	indexA         int
	indexB         int
	frameA         Transform
	frameB         Transform
	deltaCenter    Vec2
	axialMass      float64
	springSoftness softness

	enableSpring bool
	enableMotor  bool
	enableLimit  bool
}

// Point-to-point constraint
// C = pB - pA
// Cdot = vB - vA
//      = vB + cross(wB, rB) - vA - cross(wA, rA)
// J = [-E -skew(rA) E skew(rB) ]

// Identity used:
// w k % (rx i + ry j) = w * (-ry i + rx j)

// Motor constraint
// Cdot = wB - wA
// J = [0 0 -1 0 0 1]
// K = invIA + invIB

// EnableRevoluteJointSpring enables/disables the revolute joint spring
// (upstream b2RevoluteJoint_EnableSpring).
func (w *World) EnableRevoluteJointSpring(jointID JointID, enableSpring bool) {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	if enableSpring != j.revoluteJoint.enableSpring {
		j.revoluteJoint.enableSpring = enableSpring
		j.revoluteJoint.springImpulse = 0.0
	}
}

// IsRevoluteJointSpringEnabled reports whether the revolute angular spring is
// enabled (upstream b2RevoluteJoint_IsSpringEnabled).
func (w *World) IsRevoluteJointSpringEnabled(jointID JointID) bool {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.enableSpring
}

// SetRevoluteJointSpringHertz sets the revolute joint spring stiffness in
// Hertz (upstream b2RevoluteJoint_SetSpringHertz).
func (w *World) SetRevoluteJointSpringHertz(jointID JointID, hertz float64) {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	j.revoluteJoint.hertz = hertz
}

// RevoluteJointSpringHertz returns the revolute joint spring stiffness in
// Hertz (upstream b2RevoluteJoint_GetSpringHertz).
func (w *World) RevoluteJointSpringHertz(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.hertz
}

// SetRevoluteJointSpringDampingRatio sets the revolute joint spring damping
// ratio, non-dimensional (upstream b2RevoluteJoint_SetSpringDampingRatio).
func (w *World) SetRevoluteJointSpringDampingRatio(jointID JointID, dampingRatio float64) {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	j.revoluteJoint.dampingRatio = dampingRatio
}

// RevoluteJointSpringDampingRatio returns the revolute joint spring damping
// ratio (upstream b2RevoluteJoint_GetSpringDampingRatio).
func (w *World) RevoluteJointSpringDampingRatio(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.dampingRatio
}

// SetRevoluteJointTargetAngle sets the revolute joint spring target angle in
// radians (upstream b2RevoluteJoint_SetTargetAngle).
func (w *World) SetRevoluteJointTargetAngle(jointID JointID, angle float64) {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	j.revoluteJoint.targetAngle = angle
}

// RevoluteJointTargetAngle returns the revolute joint spring target angle in
// radians (upstream b2RevoluteJoint_GetTargetAngle).
func (w *World) RevoluteJointTargetAngle(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.targetAngle
}

// RevoluteJointAngle returns the current joint angle in radians relative to
// the reference angle (upstream b2RevoluteJoint_GetAngle).
func (w *World) RevoluteJointAngle(jointID JointID) float64 {
	js := w.getJointSimCheckType(jointID, RevoluteJoint)
	transformA := w.getBodyTransform(js.bodyIDA)
	transformB := w.getBodyTransform(js.bodyIDB)
	qA := MulRot(transformA.Q, js.localFrameA.Q)
	qB := MulRot(transformB.Q, js.localFrameB.Q)

	return RelativeAngle(qA, qB)
}

// EnableRevoluteJointLimit enables/disables the revolute joint limit
// (upstream b2RevoluteJoint_EnableLimit).
func (w *World) EnableRevoluteJointLimit(jointID JointID, enableLimit bool) {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	if enableLimit != j.revoluteJoint.enableLimit {
		j.revoluteJoint.enableLimit = enableLimit
		j.revoluteJoint.lowerImpulse = 0.0
		j.revoluteJoint.upperImpulse = 0.0
	}
}

// IsRevoluteJointLimitEnabled reports whether the revolute joint limit is
// enabled (upstream b2RevoluteJoint_IsLimitEnabled).
func (w *World) IsRevoluteJointLimitEnabled(jointID JointID) bool {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.enableLimit
}

// RevoluteJointLowerLimit returns the lower joint limit in radians
// (upstream b2RevoluteJoint_GetLowerLimit).
func (w *World) RevoluteJointLowerLimit(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.lowerAngle
}

// RevoluteJointUpperLimit returns the upper joint limit in radians
// (upstream b2RevoluteJoint_GetUpperLimit).
func (w *World) RevoluteJointUpperLimit(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.upperAngle
}

// SetRevoluteJointLimits sets the revolute joint limits in radians. It is
// expected that lower <= upper (upstream b2RevoluteJoint_SetLimits).
func (w *World) SetRevoluteJointLimits(jointID JointID, lower, upper float64) {
	assert(lower <= upper)
	assert(lower >= -0.99*Pi)
	assert(upper <= 0.99*Pi)

	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	if lower != j.revoluteJoint.lowerAngle || upper != j.revoluteJoint.upperAngle {
		j.revoluteJoint.lowerAngle = minFloat(lower, upper)
		j.revoluteJoint.upperAngle = maxFloat(lower, upper)
		j.revoluteJoint.lowerImpulse = 0.0
		j.revoluteJoint.upperImpulse = 0.0
	}
}

// EnableRevoluteJointMotor enables/disables the revolute joint motor
// (upstream b2RevoluteJoint_EnableMotor).
func (w *World) EnableRevoluteJointMotor(jointID JointID, enableMotor bool) {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	if enableMotor != j.revoluteJoint.enableMotor {
		j.revoluteJoint.enableMotor = enableMotor
		j.revoluteJoint.motorImpulse = 0.0
	}
}

// IsRevoluteJointMotorEnabled reports whether the revolute joint motor is
// enabled (upstream b2RevoluteJoint_IsMotorEnabled).
func (w *World) IsRevoluteJointMotorEnabled(jointID JointID) bool {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.enableMotor
}

// SetRevoluteJointMotorSpeed sets the revolute joint motor speed in radians
// per second (upstream b2RevoluteJoint_SetMotorSpeed).
func (w *World) SetRevoluteJointMotorSpeed(jointID JointID, motorSpeed float64) {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	j.revoluteJoint.motorSpeed = motorSpeed
}

// RevoluteJointMotorSpeed returns the revolute joint motor speed in radians
// per second (upstream b2RevoluteJoint_GetMotorSpeed).
func (w *World) RevoluteJointMotorSpeed(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.motorSpeed
}

// RevoluteJointMotorTorque returns the current motor torque, usually in
// Newton * meters (upstream b2RevoluteJoint_GetMotorTorque).
func (w *World) RevoluteJointMotorTorque(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return w.invH * j.revoluteJoint.motorImpulse
}

// SetRevoluteJointMaxMotorTorque sets the revolute joint maximum motor
// torque, usually in Newton * meters
// (upstream b2RevoluteJoint_SetMaxMotorTorque).
func (w *World) SetRevoluteJointMaxMotorTorque(jointID JointID, torque float64) {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	j.revoluteJoint.maxMotorTorque = torque
}

// RevoluteJointMaxMotorTorque returns the revolute joint maximum motor torque
// (upstream b2RevoluteJoint_GetMaxMotorTorque).
func (w *World) RevoluteJointMaxMotorTorque(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, RevoluteJoint)
	return j.revoluteJoint.maxMotorTorque
}

// getRevoluteJointForce returns the revolute joint constraint force
// (upstream b2GetRevoluteJointForce).
func getRevoluteJointForce(w *World, base *jointSim) Vec2 {
	return MulSV(w.invH, base.revoluteJoint.linearImpulse)
}

// getRevoluteJointTorque returns the revolute joint constraint torque
// (upstream b2GetRevoluteJointTorque).
func getRevoluteJointTorque(w *World, base *jointSim) float64 {
	revolute := &base.revoluteJoint
	return w.invH * (revolute.motorImpulse + revolute.lowerImpulse - revolute.upperImpulse)
}

// prepareRevoluteJoint mirrors b2PrepareRevoluteJoint.
func prepareRevoluteJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == RevoluteJoint)

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

	j := &base.revoluteJoint

	j.indexA = NullIndex
	if bodyA.setIndex == awakeSet {
		j.indexA = localIndexA
	}
	j.indexB = NullIndex
	if bodyB.setIndex == awakeSet {
		j.indexB = localIndexB
	}

	// Compute joint anchor frames with world space rotation, relative to
	// center of mass. Avoid round-off here as much as possible.
	// b2Vec2 pf = (xf.p - c) + rot(xf.q, f.p)
	// pf = xf.p - (xf.p + rot(xf.q, lc)) + rot(xf.q, f.p)
	// pf = rot(xf.q, f.p - lc)
	j.frameA.Q = MulRot(bodySimA.transform.Q, base.localFrameA.Q)
	j.frameA.P = RotateVector(bodySimA.transform.Q, Sub(base.localFrameA.P, bodySimA.localCenter))
	j.frameB.Q = MulRot(bodySimB.transform.Q, base.localFrameB.Q)
	j.frameB.P = RotateVector(bodySimB.transform.Q, Sub(base.localFrameB.P, bodySimB.localCenter))

	// Compute the initial center delta. Incremental position updates are
	// relative to this.
	j.deltaCenter = Sub(bodySimB.center, bodySimA.center)

	k := iA + iB
	j.axialMass = 0.0
	if k > 0.0 {
		j.axialMass = 1.0 / k
	}

	j.springSoftness = makeSoft(j.hertz, j.dampingRatio, ctx.h)

	if !ctx.enableWarmStarting {
		j.linearImpulse = Vec2Zero
		j.springImpulse = 0.0
		j.motorImpulse = 0.0
		j.lowerImpulse = 0.0
		j.upperImpulse = 0.0
	}
}

// warmStartRevoluteJoint mirrors b2WarmStartRevoluteJoint.
func warmStartRevoluteJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == RevoluteJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.revoluteJoint
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

	axialImpulse := j.springImpulse + j.motorImpulse + j.lowerImpulse - j.upperImpulse

	if stateA.flags&dynamicFlag != 0 {
		stateA.linearVelocity = MulSub(stateA.linearVelocity, mA, j.linearImpulse)
		// wA -= iA * (cross(rA, linearImpulse) + axialImpulse)
		stateA.angularVelocity -= float64(iA * (Cross(rA, j.linearImpulse) + axialImpulse))
	}

	if stateB.flags&dynamicFlag != 0 {
		stateB.linearVelocity = MulAdd(stateB.linearVelocity, mB, j.linearImpulse)
		// wB += iB * (cross(rB, linearImpulse) + axialImpulse)
		stateB.angularVelocity += float64(iB * (Cross(rB, j.linearImpulse) + axialImpulse))
	}
}

// solveRevoluteJoint mirrors b2SolveRevoluteJoint.
func solveRevoluteJoint(base *jointSim, ctx *stepContext, useBias bool) {
	assert(base.jointType == RevoluteJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.revoluteJoint

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

	qA := MulRot(stateA.deltaRotation, j.frameA.Q)
	qB := MulRot(stateB.deltaRotation, j.frameB.Q)
	relQ := InvMulRot(qA, qB)

	fixedRotation := iA+iB == 0.0

	// Solve spring.
	if j.enableSpring && !fixedRotation {
		jointAngle := RotGetAngle(relQ)
		jointAngleDelta := UnwindAngle(jointAngle - j.targetAngle)

		c := jointAngleDelta
		bias := float64(j.springSoftness.biasRate * c)
		massScale := j.springSoftness.massScale
		impulseScale := j.springSoftness.impulseScale

		cDot := wB - wA
		// impulse = -massScale * axialMass * (Cdot + bias) - impulseScale * springImpulse
		impulse := cross2(-massScale*j.axialMass, cDot+bias, impulseScale, j.springImpulse)
		j.springImpulse += impulse

		// wA -= iA * impulse
		wA -= float64(iA * impulse)
		// wB += iB * impulse
		wB += float64(iB * impulse)
	}

	// Solve motor constraint.
	if j.enableMotor && !fixedRotation {
		cDot := wB - wA - j.motorSpeed
		impulse := float64(-j.axialMass * cDot)
		oldImpulse := j.motorImpulse
		maxImpulse := ctx.h * j.maxMotorTorque
		j.motorImpulse = clampFloat(j.motorImpulse+impulse, -maxImpulse, maxImpulse)
		impulse = j.motorImpulse - oldImpulse

		wA -= float64(iA * impulse)
		wB += float64(iB * impulse)
	}

	if j.enableLimit && !fixedRotation {
		jointAngle := RotGetAngle(relQ)

		// Lower limit
		{
			c := jointAngle - j.lowerAngle
			bias := 0.0
			massScale := 1.0
			impulseScale := 0.0
			if c > 0.0 {
				// speculation
				bias = c * ctx.invH
			} else if useBias {
				bias = base.constraintSoftness.biasRate * c
				massScale = base.constraintSoftness.massScale
				impulseScale = base.constraintSoftness.impulseScale
			}

			cDot := wB - wA
			oldImpulse := j.lowerImpulse
			// impulse = -massScale * axialMass * (Cdot + bias) - impulseScale * oldImpulse
			impulse := cross2(-massScale*j.axialMass, cDot+bias, impulseScale, oldImpulse)
			j.lowerImpulse = maxFloat(oldImpulse+impulse, 0.0)
			impulse = j.lowerImpulse - oldImpulse

			wA -= float64(iA * impulse)
			wB += float64(iB * impulse)
		}

		// Upper limit
		// Note: signs are flipped to keep C positive when the constraint is
		// satisfied. This also keeps the impulse positive when the limit is
		// active.
		{
			c := j.upperAngle - jointAngle
			bias := 0.0
			massScale := 1.0
			impulseScale := 0.0
			if c > 0.0 {
				// speculation
				bias = c * ctx.invH
			} else if useBias {
				bias = base.constraintSoftness.biasRate * c
				massScale = base.constraintSoftness.massScale
				impulseScale = base.constraintSoftness.impulseScale
			}

			// sign flipped on Cdot
			cDot := wA - wB
			oldImpulse := j.upperImpulse
			// impulse = -massScale * axialMass * (Cdot + bias) - impulseScale * oldImpulse
			impulse := cross2(-massScale*j.axialMass, cDot+bias, impulseScale, oldImpulse)
			j.upperImpulse = maxFloat(oldImpulse+impulse, 0.0)
			impulse = j.upperImpulse - oldImpulse

			// sign flipped on applied impulse
			wA += float64(iA * impulse)
			wB -= float64(iB * impulse)
		}
	}

	// Solve point-to-point constraint
	{
		// J = [-I -r1_skew I r2_skew]
		// r_skew = [-ry; rx]
		// K = [ mA+r1y^2*iA+mB+r2y^2*iB,  -r1y*iA*r1x-r2y*iB*r2x]
		//     [  -r1y*iA*r1x-r2y*iB*r2x, mA+r1x^2*iA+mB+r2x^2*iB]

		// current anchors
		rA := RotateVector(stateA.deltaRotation, j.frameA.P)
		rB := RotateVector(stateB.deltaRotation, j.frameB.P)

		cDot := Sub(Add(vB, CrossSV(wB, rB)), Add(vA, CrossSV(wA, rA)))

		bias := Vec2Zero
		massScale := 1.0
		impulseScale := 0.0
		if useBias {
			dcA := stateA.deltaPosition
			dcB := stateB.deltaPosition

			separation := Add(Add(Sub(dcB, dcA), Sub(rB, rA)), j.deltaCenter)
			bias = MulSV(base.constraintSoftness.biasRate, separation)
			massScale = base.constraintSoftness.massScale
			impulseScale = base.constraintSoftness.impulseScale
		}

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
		j.linearImpulse.X += impulse.X
		j.linearImpulse.Y += impulse.Y

		vA = MulSub(vA, mA, impulse)
		// wA -= iA * cross(rA, impulse)
		wA -= float64(iA * Cross(rA, impulse))
		vB = MulAdd(vB, mB, impulse)
		// wB += iB * cross(rB, impulse)
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

// drawRevoluteJoint renders a revolute joint (upstream b2DrawRevoluteJoint).
func drawRevoluteJoint(draw *DebugDraw, base *jointSim, transformA, transformB Transform, drawScale float64) {
	assert(base.jointType == RevoluteJoint)

	j := &base.revoluteJoint

	frameA := MulTransforms(transformA, base.localFrameA)
	frameB := MulTransforms(transformB, base.localFrameB)

	radius := 0.25 * drawScale
	draw.DrawCircleFcn(frameB.P, radius, ColorGray, draw.Context)

	rx := Vec2{X: radius, Y: 0.0}
	r := RotateVector(frameA.Q, rx)
	draw.DrawLineFcn(frameA.P, Add(frameA.P, r), ColorGray, draw.Context)

	r = RotateVector(frameB.Q, rx)
	draw.DrawLineFcn(frameB.P, Add(frameB.P, r), ColorBlue, draw.Context)

	if draw.DrawJointExtras {
		jointAngle := RelativeAngle(frameA.Q, frameB.Q)
		draw.DrawStringFcn(Add(frameA.P, r), " "+strconv.FormatFloat(180.0*jointAngle/Pi, 'f', 1, 64)+" deg", ColorWhite, draw.Context)
	}

	if j.enableLimit {
		rotLo := MulRot(frameA.Q, MakeRot(j.lowerAngle))
		rlo := RotateVector(rotLo, rx)

		rotHi := MulRot(frameA.Q, MakeRot(j.upperAngle))
		rhi := RotateVector(rotHi, rx)

		draw.DrawLineFcn(frameB.P, Add(frameB.P, rlo), ColorGreen, draw.Context)
		draw.DrawLineFcn(frameB.P, Add(frameB.P, rhi), ColorRed, draw.Context)
	}

	if j.enableSpring {
		q := MulRot(frameA.Q, MakeRot(j.targetAngle))
		v := RotateVector(q, rx)
		draw.DrawLineFcn(frameB.P, Add(frameB.P, v), ColorViolet, draw.Context)
	}

	color := ColorGold
	draw.DrawLineFcn(transformA.P, frameA.P, color, draw.Context)
	draw.DrawLineFcn(frameA.P, frameB.P, color, draw.Context)
	draw.DrawLineFcn(transformB.P, frameB.P, color, draw.Context)
}
