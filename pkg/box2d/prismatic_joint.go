// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/prismatic_joint.c
// (plus the b2PrismaticJoint struct from src/joint.h).
//
// Public API mapping: b2PrismaticJoint_EnableSpring →
// (*World).EnablePrismaticJointSpring, b2PrismaticJoint_GetTranslation →
// (*World).PrismaticJointTranslation, and so on (getters are Get-less).
//

package box2d

// prismaticJoint is the prismatic joint solver state (upstream
// b2PrismaticJoint in joint.h). Lives in the jointSim per-type union.
type prismaticJoint struct {
	impulse           Vec2
	springImpulse     float64
	motorImpulse      float64
	lowerImpulse      float64
	upperImpulse      float64
	hertz             float64
	dampingRatio      float64
	targetTranslation float64
	maxMotorForce     float64
	motorSpeed        float64
	lowerTranslation  float64
	upperTranslation  float64

	indexA         int
	indexB         int
	frameA         Transform
	frameB         Transform
	deltaCenter    Vec2
	springSoftness softness

	enableSpring bool
	enableLimit  bool
	enableMotor  bool
}

// EnablePrismaticJointSpring enables/disables the prismatic joint spring
// (upstream b2PrismaticJoint_EnableSpring).
func (w *World) EnablePrismaticJointSpring(jointID JointID, enableSpring bool) {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	if enableSpring != j.prismaticJoint.enableSpring {
		j.prismaticJoint.enableSpring = enableSpring
		j.prismaticJoint.springImpulse = 0.0
	}
}

// IsPrismaticJointSpringEnabled reports whether the prismatic joint spring is
// enabled (upstream b2PrismaticJoint_IsSpringEnabled).
func (w *World) IsPrismaticJointSpringEnabled(jointID JointID) bool {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.enableSpring
}

// SetPrismaticJointSpringHertz sets the prismatic joint spring stiffness in
// Hertz (upstream b2PrismaticJoint_SetSpringHertz).
func (w *World) SetPrismaticJointSpringHertz(jointID JointID, hertz float64) {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	j.prismaticJoint.hertz = hertz
}

// PrismaticJointSpringHertz returns the prismatic joint spring stiffness in
// Hertz (upstream b2PrismaticJoint_GetSpringHertz).
func (w *World) PrismaticJointSpringHertz(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.hertz
}

// SetPrismaticJointSpringDampingRatio sets the prismatic joint spring damping
// ratio, non-dimensional
// (upstream b2PrismaticJoint_SetSpringDampingRatio).
func (w *World) SetPrismaticJointSpringDampingRatio(jointID JointID, dampingRatio float64) {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	j.prismaticJoint.dampingRatio = dampingRatio
}

// PrismaticJointSpringDampingRatio returns the prismatic joint spring damping
// ratio (upstream b2PrismaticJoint_GetSpringDampingRatio).
func (w *World) PrismaticJointSpringDampingRatio(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.dampingRatio
}

// SetPrismaticJointTargetTranslation sets the prismatic joint spring target
// translation in meters
// (upstream b2PrismaticJoint_SetTargetTranslation).
func (w *World) SetPrismaticJointTargetTranslation(jointID JointID, translation float64) {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	j.prismaticJoint.targetTranslation = translation
}

// PrismaticJointTargetTranslation returns the prismatic joint spring target
// translation in meters
// (upstream b2PrismaticJoint_GetTargetTranslation).
func (w *World) PrismaticJointTargetTranslation(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.targetTranslation
}

// EnablePrismaticJointLimit enables/disables the prismatic joint limit
// (upstream b2PrismaticJoint_EnableLimit).
func (w *World) EnablePrismaticJointLimit(jointID JointID, enableLimit bool) {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	if enableLimit != j.prismaticJoint.enableLimit {
		j.prismaticJoint.enableLimit = enableLimit
		j.prismaticJoint.lowerImpulse = 0.0
		j.prismaticJoint.upperImpulse = 0.0
	}
}

// IsPrismaticJointLimitEnabled reports whether the prismatic joint limit is
// enabled (upstream b2PrismaticJoint_IsLimitEnabled).
func (w *World) IsPrismaticJointLimitEnabled(jointID JointID) bool {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.enableLimit
}

// PrismaticJointLowerLimit returns the lower joint limit in meters
// (upstream b2PrismaticJoint_GetLowerLimit).
func (w *World) PrismaticJointLowerLimit(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.lowerTranslation
}

// PrismaticJointUpperLimit returns the upper joint limit in meters
// (upstream b2PrismaticJoint_GetUpperLimit).
func (w *World) PrismaticJointUpperLimit(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.upperTranslation
}

// SetPrismaticJointLimits sets the joint limits in meters. It is expected
// that lower <= upper (upstream b2PrismaticJoint_SetLimits).
func (w *World) SetPrismaticJointLimits(jointID JointID, lower, upper float64) {
	assert(lower <= upper)

	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	if lower != j.prismaticJoint.lowerTranslation || upper != j.prismaticJoint.upperTranslation {
		j.prismaticJoint.lowerTranslation = minFloat(lower, upper)
		j.prismaticJoint.upperTranslation = maxFloat(lower, upper)
		j.prismaticJoint.lowerImpulse = 0.0
		j.prismaticJoint.upperImpulse = 0.0
	}
}

// EnablePrismaticJointMotor enables/disables the prismatic joint motor
// (upstream b2PrismaticJoint_EnableMotor).
func (w *World) EnablePrismaticJointMotor(jointID JointID, enableMotor bool) {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	if enableMotor != j.prismaticJoint.enableMotor {
		j.prismaticJoint.enableMotor = enableMotor
		j.prismaticJoint.motorImpulse = 0.0
	}
}

// IsPrismaticJointMotorEnabled reports whether the prismatic joint motor is
// enabled (upstream b2PrismaticJoint_IsMotorEnabled).
func (w *World) IsPrismaticJointMotorEnabled(jointID JointID) bool {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.enableMotor
}

// SetPrismaticJointMotorSpeed sets the prismatic joint motor speed, usually in
// meters per second (upstream b2PrismaticJoint_SetMotorSpeed).
func (w *World) SetPrismaticJointMotorSpeed(jointID JointID, motorSpeed float64) {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	j.prismaticJoint.motorSpeed = motorSpeed
}

// PrismaticJointMotorSpeed returns the prismatic joint motor speed, usually in
// meters per second (upstream b2PrismaticJoint_GetMotorSpeed).
func (w *World) PrismaticJointMotorSpeed(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.motorSpeed
}

// PrismaticJointMotorForce returns the current motor force, usually in Newtons
// (upstream b2PrismaticJoint_GetMotorForce).
func (w *World) PrismaticJointMotorForce(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, PrismaticJoint)
	return w.invH * base.prismaticJoint.motorImpulse
}

// SetPrismaticJointMaxMotorForce sets the prismatic joint maximum motor force,
// usually in Newtons (upstream b2PrismaticJoint_SetMaxMotorForce).
func (w *World) SetPrismaticJointMaxMotorForce(jointID JointID, force float64) {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	j.prismaticJoint.maxMotorForce = force
}

// PrismaticJointMaxMotorForce returns the prismatic joint maximum motor force
// (upstream b2PrismaticJoint_GetMaxMotorForce).
func (w *World) PrismaticJointMaxMotorForce(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, PrismaticJoint)
	return j.prismaticJoint.maxMotorForce
}

// PrismaticJointTranslation returns the current joint translation, usually in
// meters (upstream b2PrismaticJoint_GetTranslation).
func (w *World) PrismaticJointTranslation(jointID JointID) float64 {
	js := w.getJointSimCheckType(jointID, PrismaticJoint)
	transformA := w.getBodyTransform(js.bodyIDA)
	transformB := w.getBodyTransform(js.bodyIDB)

	localAxisA := RotateVector(js.localFrameA.Q, Vec2{X: 1.0, Y: 0.0})
	axisA := RotateVector(transformA.Q, localAxisA)
	pA := TransformPoint(transformA, js.localFrameA.P)
	pB := TransformPoint(transformB, js.localFrameB.P)
	d := Sub(pB, pA)
	translation := Dot(d, axisA)
	return translation
}

// PrismaticJointSpeed returns the current joint translation speed, usually in
// meters per second (upstream b2PrismaticJoint_GetSpeed).
func (w *World) PrismaticJointSpeed(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, PrismaticJoint)

	bodyA := &w.bodies[base.bodyIDA]
	bodyB := &w.bodies[base.bodyIDB]
	bodySimA := w.getBodySim(bodyA)
	bodySimB := w.getBodySim(bodyB)
	bodyStateA := w.getBodyState(bodyA)
	bodyStateB := w.getBodyState(bodyB)

	transformA := bodySimA.transform
	transformB := bodySimB.transform

	localAxisA := RotateVector(base.localFrameA.Q, Vec2{X: 1.0, Y: 0.0})
	axisA := RotateVector(transformA.Q, localAxisA)
	cA := bodySimA.center
	cB := bodySimB.center
	rA := RotateVector(transformA.Q, Sub(base.localFrameA.P, bodySimA.localCenter))
	rB := RotateVector(transformB.Q, Sub(base.localFrameB.P, bodySimB.localCenter))

	d := Add(Sub(cB, cA), Sub(rB, rA))

	vA := Vec2Zero
	if bodyStateA != nil {
		vA = bodyStateA.linearVelocity
	}
	vB := Vec2Zero
	if bodyStateB != nil {
		vB = bodyStateB.linearVelocity
	}
	wA := 0.0
	if bodyStateA != nil {
		wA = bodyStateA.angularVelocity
	}
	wB := 0.0
	if bodyStateB != nil {
		wB = bodyStateB.angularVelocity
	}

	vRel := Sub(Add(vB, CrossSV(wB, rB)), Add(vA, CrossSV(wA, rA)))
	speed := Dot(d, CrossSV(wA, axisA)) + Dot(axisA, vRel)
	return speed
}

// getPrismaticJointForce returns the prismatic joint constraint force
// (upstream b2GetPrismaticJointForce).
func getPrismaticJointForce(w *World, base *jointSim) Vec2 {
	idA := base.bodyIDA
	transformA := w.getBodyTransform(idA)

	j := &base.prismaticJoint

	localAxisA := RotateVector(base.localFrameA.Q, Vec2{X: 1.0, Y: 0.0})
	axisA := RotateVector(transformA.Q, localAxisA)
	perpA := LeftPerp(axisA)

	invH := w.invH
	perpForce := invH * j.impulse.X
	axialForce := invH * (j.motorImpulse + j.lowerImpulse - j.upperImpulse)

	force := Add(MulSV(perpForce, perpA), MulSV(axialForce, axisA))
	return force
}

// getPrismaticJointTorque returns the prismatic joint constraint torque
// (upstream b2GetPrismaticJointTorque).
func getPrismaticJointTorque(w *World, base *jointSim) float64 {
	return w.invH * base.prismaticJoint.impulse.Y
}

// Linear constraint (point-to-line)
// d = pB - pA = xB + rB - xA - rA
// C = dot(perp, d)
// Cdot = dot(d, cross(wA, perp)) + dot(perp, vB + cross(wB, rB) - vA - cross(wA, rA))
//      = -dot(perp, vA) - dot(cross(rA + d, perp), wA) + dot(perp, vB) + dot(cross(rB, perp), vB)
// J = [-perp, -cross(rA + d, perp), perp, cross(rB, perp)]
//
// Angular constraint
// C = aB - aA + a_initial
// Cdot = wB - wA
// J = [0 0 -1 0 0 1]
//
// K = J * invM * JT
//
// J = [-a -sA a sB]
//     [0  -1  0  1]
// a = perp
// sA = cross(rA + d, a) = cross(pB - xA, a)
// sB = cross(rB, a) = cross(pB - xB, a)

// Motor/Limit linear constraint
// C = dot(axA, d)
// Cdot = -dot(axA, vA) - dot(cross(rA + d, axA), wA) + dot(axA, vB) + dot(cross(rB, axA), vB)
// J = [-axA -cross(rA + d, axA) axA cross(rB, ax1)]

// Predictive limit is applied even when the limit is not active.
// Prevents a constraint speed that can lead to a constraint error in one time
// step. Want C2 = C1 + h * Cdot >= 0
// Or:
// Cdot + C1/h >= 0
// I do not apply a negative constraint error because that is handled in
// position correction. So:
// Cdot + max(C1, 0)/h >= 0

// Block Solver
// We develop a block solver that includes the angular and linear constraints.
// This makes the limit stiffer.
//
// The Jacobian has 2 rows:
// J = [-uT -s1 uT s2] // linear
//     [0   -1   0  1] // angular
//
// u = perp
// s1 = cross(d + r1, u), s2 = cross(r2, u)
// a1 = cross(d + r1, v), a2 = cross(r2, v)

// preparePrismaticJoint mirrors b2PreparePrismaticJoint.
func preparePrismaticJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == PrismaticJoint)

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

	j := &base.prismaticJoint

	j.indexA = NullIndex
	if bodyA.setIndex == awakeSet {
		j.indexA = localIndexA
	}
	j.indexB = NullIndex
	if bodyB.setIndex == awakeSet {
		j.indexB = localIndexB
	}

	// Compute joint anchor frames with world space rotation, relative to
	// center of mass
	j.frameA.Q = MulRot(bodySimA.transform.Q, base.localFrameA.Q)
	j.frameA.P = RotateVector(bodySimA.transform.Q, Sub(base.localFrameA.P, bodySimA.localCenter))
	j.frameB.Q = MulRot(bodySimB.transform.Q, base.localFrameB.Q)
	j.frameB.P = RotateVector(bodySimB.transform.Q, Sub(base.localFrameB.P, bodySimB.localCenter))

	// Compute the initial center delta. Incremental position updates are
	// relative to this.
	j.deltaCenter = Sub(bodySimB.center, bodySimA.center)

	j.springSoftness = makeSoft(j.hertz, j.dampingRatio, ctx.h)

	if !ctx.enableWarmStarting {
		j.impulse = Vec2Zero
		j.springImpulse = 0.0
		j.motorImpulse = 0.0
		j.lowerImpulse = 0.0
		j.upperImpulse = 0.0
	}
}

// warmStartPrismaticJoint mirrors b2WarmStartPrismaticJoint.
func warmStartPrismaticJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == PrismaticJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.prismaticJoint

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

	d := Add(Add(Sub(stateB.deltaPosition, stateA.deltaPosition), j.deltaCenter), Sub(rB, rA))

	axisA := RotateVector(j.frameA.Q, Vec2{X: 1.0, Y: 0.0})
	axisA = RotateVector(stateA.deltaRotation, axisA)

	// impulse is applied at anchor point on body B
	a1 := Cross(Add(rA, d), axisA)
	a2 := Cross(rB, axisA)
	axialImpulse := j.springImpulse + j.motorImpulse + j.lowerImpulse - j.upperImpulse

	// perpendicular constraint
	perpA := LeftPerp(axisA)
	s1 := Cross(Add(rA, d), perpA)
	s2 := Cross(rB, perpA)
	perpImpulse := j.impulse.X
	angleImpulse := j.impulse.Y

	p := Add(MulSV(axialImpulse, axisA), MulSV(perpImpulse, perpA))
	// LA = axialImpulse * a1 + perpImpulse * s1 + angleImpulse
	la := dot2(axialImpulse, a1, perpImpulse, s1) + angleImpulse
	// LB = axialImpulse * a2 + perpImpulse * s2 + angleImpulse
	lb := dot2(axialImpulse, a2, perpImpulse, s2) + angleImpulse

	if stateA.flags&dynamicFlag != 0 {
		stateA.linearVelocity = MulSub(stateA.linearVelocity, mA, p)
		// wA -= iA * LA
		stateA.angularVelocity -= float64(iA * la)
	}

	if stateB.flags&dynamicFlag != 0 {
		stateB.linearVelocity = MulAdd(stateB.linearVelocity, mB, p)
		// wB += iB * LB
		stateB.angularVelocity += float64(iB * lb)
	}
}

// solvePrismaticJoint mirrors b2SolvePrismaticJoint.
func solvePrismaticJoint(base *jointSim, ctx *stepContext, useBias bool) {
	assert(base.jointType == PrismaticJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.prismaticJoint

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

	// current anchors
	rA := RotateVector(stateA.deltaRotation, j.frameA.P)
	rB := RotateVector(stateB.deltaRotation, j.frameB.P)

	d := Add(Add(Sub(stateB.deltaPosition, stateA.deltaPosition), j.deltaCenter), Sub(rB, rA))

	axisA := RotateVector(j.frameA.Q, Vec2{X: 1.0, Y: 0.0})
	axisA = RotateVector(stateA.deltaRotation, axisA)
	translation := Dot(axisA, d)

	// These scalars are for torques generated by axial forces
	a1 := Cross(Add(rA, d), axisA)
	a2 := Cross(rB, axisA)

	// k = mA + mB + iA * a1 * a1 + iB * a2 * a2
	k := mA + mB + float64(iA*a1*a1) + float64(iB*a2*a2)
	axialMass := 0.0
	if k > 0.0 {
		axialMass = 1.0 / k
	}

	soft := base.constraintSoftness

	// spring constraint
	if j.enableSpring {
		// This is a real spring and should be applied even during relax
		c := translation - j.targetTranslation
		bias := float64(j.springSoftness.biasRate * c)
		massScale := j.springSoftness.massScale
		impulseScale := j.springSoftness.impulseScale

		// Cdot = dot(axisA, vB - vA) + a2 * wB - a1 * wA
		cDot := Dot(axisA, Sub(vB, vA)) + float64(a2*wB) - float64(a1*wA)
		// deltaImpulse = -massScale * axialMass * (Cdot + bias) - impulseScale * springImpulse
		deltaImpulse := cross2(-massScale*axialMass, cDot+bias, impulseScale, j.springImpulse)
		j.springImpulse += deltaImpulse

		p := MulSV(deltaImpulse, axisA)
		la := deltaImpulse * a1
		lb := deltaImpulse * a2

		vA = MulSub(vA, mA, p)
		wA -= float64(iA * la)
		vB = MulAdd(vB, mB, p)
		wB += float64(iB * lb)
	}

	// Solve motor constraint
	if j.enableMotor {
		// Cdot = dot(axisA, vB - vA) + a2 * wB - a1 * wA
		cDot := Dot(axisA, Sub(vB, vA)) + float64(a2*wB) - float64(a1*wA)
		impulse := float64(axialMass * (j.motorSpeed - cDot))
		oldImpulse := j.motorImpulse
		maxImpulse := ctx.h * j.maxMotorForce
		j.motorImpulse = clampFloat(j.motorImpulse+impulse, -maxImpulse, maxImpulse)
		impulse = j.motorImpulse - oldImpulse

		p := MulSV(impulse, axisA)
		la := impulse * a1
		lb := impulse * a2

		vA = MulSub(vA, mA, p)
		wA -= float64(iA * la)
		vB = MulAdd(vB, mB, p)
		wB += float64(iB * lb)
	}

	if j.enableLimit {
		// Clamp the speculative distance to a reasonable value
		speculativeDistance := 0.25 * (j.upperTranslation - j.lowerTranslation)

		// Lower limit
		{
			c := translation - j.lowerTranslation

			if c < speculativeDistance {
				bias := 0.0
				massScale := 1.0
				impulseScale := 0.0

				if c > 0.0 {
					// speculation
					safe := GetLengthUnitsPerMeter()
					bias = minFloat(c, safe) * ctx.invH
				} else if useBias {
					bias = soft.biasRate * c
					massScale = soft.massScale
					impulseScale = soft.impulseScale
				}

				oldImpulse := j.lowerImpulse
				// Cdot = dot(axisA, vB - vA) + a2 * wB - a1 * wA
				cDot := Dot(axisA, Sub(vB, vA)) + float64(a2*wB) - float64(a1*wA)
				// deltaImpulse = -axialMass * massScale * (Cdot + bias) - impulseScale * oldImpulse
				deltaImpulse := cross2(-axialMass*massScale, cDot+bias, impulseScale, oldImpulse)
				j.lowerImpulse = maxFloat(oldImpulse+deltaImpulse, 0.0)
				deltaImpulse = j.lowerImpulse - oldImpulse

				p := MulSV(deltaImpulse, axisA)
				la := deltaImpulse * a1
				lb := deltaImpulse * a2

				vA = MulSub(vA, mA, p)
				wA -= float64(iA * la)
				vB = MulAdd(vB, mB, p)
				wB += float64(iB * lb)
			} else {
				j.lowerImpulse = 0.0
			}
		}

		// Upper limit
		// Note: signs are flipped to keep C positive when the constraint is
		// satisfied. This also keeps the impulse positive when the limit is
		// active.
		{
			// sign flipped
			c := j.upperTranslation - translation

			if c < speculativeDistance {
				bias := 0.0
				massScale := 1.0
				impulseScale := 0.0

				if c > 0.0 {
					// speculation
					safe := GetLengthUnitsPerMeter()
					bias = minFloat(c, safe) * ctx.invH
				} else if useBias {
					bias = soft.biasRate * c
					massScale = soft.massScale
					impulseScale = soft.impulseScale
				}

				oldImpulse := j.upperImpulse

				// sign flipped
				// Cdot = dot(axisA, vA - vB) + a1 * wA - a2 * wB
				cDot := Dot(axisA, Sub(vA, vB)) + float64(a1*wA) - float64(a2*wB)
				// deltaImpulse = -axialMass * massScale * (Cdot + bias) - impulseScale * oldImpulse
				deltaImpulse := cross2(-axialMass*massScale, cDot+bias, impulseScale, oldImpulse)
				j.upperImpulse = maxFloat(oldImpulse+deltaImpulse, 0.0)
				deltaImpulse = j.upperImpulse - oldImpulse

				p := MulSV(deltaImpulse, axisA)
				la := deltaImpulse * a1
				lb := deltaImpulse * a2

				// sign flipped
				vA = MulAdd(vA, mA, p)
				wA += float64(iA * la)
				vB = MulSub(vB, mB, p)
				wB -= float64(iB * lb)
			} else {
				j.upperImpulse = 0.0
			}
		}
	}

	// Solve the prismatic constraint in block form
	{
		perpA := LeftPerp(axisA)

		// These scalars are for torques generated by the perpendicular
		// constraint force
		s1 := Cross(Add(d, rA), perpA)
		s2 := Cross(rB, perpA)

		var cDot Vec2
		// Cdot.x = dot(perpA, vB - vA) + s2 * wB - s1 * wA
		cDot.X = Dot(perpA, Sub(vB, vA)) + float64(s2*wB) - float64(s1*wA)
		cDot.Y = wB - wA

		bias := Vec2Zero
		massScale := 1.0
		impulseScale := 0.0
		if useBias {
			var c Vec2
			c.X = Dot(perpA, d)
			c.Y = RotGetAngle(relQ)

			bias = MulSV(soft.biasRate, c)
			massScale = soft.massScale
			impulseScale = soft.impulseScale
		}

		// k11 = mA + mB + iA * s1 * s1 + iB * s2 * s2
		k11 := mA + mB + float64(iA*s1*s1) + float64(iB*s2*s2)
		// k12 = iA * s1 + iB * s2
		k12 := dot2(iA, s1, iB, s2)
		k22 := iA + iB
		if k22 == 0.0 {
			// For bodies with fixed rotation.
			k22 = 1.0
		}

		kMat := Mat22{CX: Vec2{X: k11, Y: k12}, CY: Vec2{X: k12, Y: k22}}

		b := Solve22(kMat, Add(cDot, bias))
		var deltaImpulse Vec2
		// deltaImpulse.x = -massScale * b.x - impulseScale * impulse.x
		deltaImpulse.X = cross2(-massScale, b.X, impulseScale, j.impulse.X)
		// deltaImpulse.y = -massScale * b.y - impulseScale * impulse.y
		deltaImpulse.Y = cross2(-massScale, b.Y, impulseScale, j.impulse.Y)

		j.impulse.X += deltaImpulse.X
		j.impulse.Y += deltaImpulse.Y

		p := MulSV(deltaImpulse.X, perpA)
		// LA = deltaImpulse.x * s1 + deltaImpulse.y
		la := mulAdd(deltaImpulse.X, s1, deltaImpulse.Y)
		// LB = deltaImpulse.x * s2 + deltaImpulse.y
		lb := mulAdd(deltaImpulse.X, s2, deltaImpulse.Y)

		vA = MulSub(vA, mA, p)
		wA -= float64(iA * la)
		vB = MulAdd(vB, mB, p)
		wB += float64(iB * lb)
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

// drawPrismaticJoint renders a prismatic joint (upstream b2DrawPrismaticJoint).
func drawPrismaticJoint(draw *DebugDraw, base *jointSim, transformA, transformB Transform, drawScale float64) {
	assert(base.jointType == PrismaticJoint)

	j := &base.prismaticJoint

	frameA := MulTransforms(transformA, base.localFrameA)
	frameB := MulTransforms(transformB, base.localFrameB)
	axisA := RotateVector(frameA.Q, Vec2{X: 1.0, Y: 0.0})

	draw.DrawLineFcn(frameA.P, frameB.P, ColorDimGray, draw.Context)

	if j.enableLimit {
		b := 0.25 * drawScale
		lower := MulAdd(frameA.P, j.lowerTranslation, axisA)
		upper := MulAdd(frameA.P, j.upperTranslation, axisA)
		perp := LeftPerp(axisA)

		draw.DrawLineFcn(lower, upper, ColorGray, draw.Context)
		draw.DrawLineFcn(MulSub(lower, b, perp), MulAdd(lower, b, perp), ColorGreen, draw.Context)
		draw.DrawLineFcn(MulSub(upper, b, perp), MulAdd(upper, b, perp), ColorRed, draw.Context)
	} else {
		draw.DrawLineFcn(MulSub(frameA.P, 1.0, axisA), MulAdd(frameA.P, 1.0, axisA), ColorGray, draw.Context)
	}

	if j.enableSpring {
		p := MulAdd(frameA.P, j.targetTranslation, axisA)
		draw.DrawPointFcn(p, 8.0, ColorViolet, draw.Context)
	}

	draw.DrawPointFcn(frameA.P, 5.0, ColorGray, draw.Context)
	draw.DrawPointFcn(frameB.P, 5.0, ColorBlue, draw.Context)
}
