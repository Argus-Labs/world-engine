// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/wheel_joint.c
// (plus the b2WheelJoint struct from src/joint.h).
//
// Public API mapping: b2WheelJoint_EnableSpring →
// (*World).EnableWheelJointSpring, b2WheelJoint_GetMotorTorque →
// (*World).WheelJointMotorTorque, and so on (getters are Get-less).
//

package box2d

// wheelJoint is the wheel joint solver state (upstream b2WheelJoint in
// joint.h). Lives in the jointSim per-type union.
type wheelJoint struct {
	perpImpulse      float64
	motorImpulse     float64
	springImpulse    float64
	lowerImpulse     float64
	upperImpulse     float64
	maxMotorTorque   float64
	motorSpeed       float64
	lowerTranslation float64
	upperTranslation float64
	hertz            float64
	dampingRatio     float64

	indexA         int
	indexB         int
	frameA         Transform
	frameB         Transform
	deltaCenter    Vec2
	perpMass       float64
	motorMass      float64
	axialMass      float64
	springSoftness softness

	enableSpring bool
	enableMotor  bool
	enableLimit  bool
}

// EnableWheelJointSpring enables/disables the wheel joint spring
// (upstream b2WheelJoint_EnableSpring).
func (w *World) EnableWheelJointSpring(jointID JointID, enableSpring bool) {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	if enableSpring != j.wheelJoint.enableSpring {
		j.wheelJoint.enableSpring = enableSpring
		j.wheelJoint.springImpulse = 0.0
	}
}

// IsWheelJointSpringEnabled reports whether the wheel joint spring is enabled
// (upstream b2WheelJoint_IsSpringEnabled).
func (w *World) IsWheelJointSpringEnabled(jointID JointID) bool {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return j.wheelJoint.enableSpring
}

// SetWheelJointSpringHertz sets the wheel joint spring stiffness in Hertz
// (upstream b2WheelJoint_SetSpringHertz).
func (w *World) SetWheelJointSpringHertz(jointID JointID, hertz float64) {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	j.wheelJoint.hertz = hertz
}

// WheelJointSpringHertz returns the wheel joint spring stiffness in Hertz
// (upstream b2WheelJoint_GetSpringHertz).
func (w *World) WheelJointSpringHertz(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return j.wheelJoint.hertz
}

// SetWheelJointSpringDampingRatio sets the wheel joint spring damping ratio,
// non-dimensional (upstream b2WheelJoint_SetSpringDampingRatio).
func (w *World) SetWheelJointSpringDampingRatio(jointID JointID, dampingRatio float64) {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	j.wheelJoint.dampingRatio = dampingRatio
}

// WheelJointSpringDampingRatio returns the wheel joint spring damping ratio
// (upstream b2WheelJoint_GetSpringDampingRatio).
func (w *World) WheelJointSpringDampingRatio(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return j.wheelJoint.dampingRatio
}

// EnableWheelJointLimit enables/disables the wheel joint limit
// (upstream b2WheelJoint_EnableLimit).
func (w *World) EnableWheelJointLimit(jointID JointID, enableLimit bool) {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	if j.wheelJoint.enableLimit != enableLimit {
		j.wheelJoint.lowerImpulse = 0.0
		j.wheelJoint.upperImpulse = 0.0
		j.wheelJoint.enableLimit = enableLimit
	}
}

// IsWheelJointLimitEnabled reports whether the wheel joint limit is enabled
// (upstream b2WheelJoint_IsLimitEnabled).
func (w *World) IsWheelJointLimitEnabled(jointID JointID) bool {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return j.wheelJoint.enableLimit
}

// WheelJointLowerLimit returns the lower joint limit in meters
// (upstream b2WheelJoint_GetLowerLimit).
func (w *World) WheelJointLowerLimit(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return j.wheelJoint.lowerTranslation
}

// WheelJointUpperLimit returns the upper joint limit in meters
// (upstream b2WheelJoint_GetUpperLimit).
func (w *World) WheelJointUpperLimit(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return j.wheelJoint.upperTranslation
}

// SetWheelJointLimits sets the joint limits in meters. It is expected that
// lower <= upper (upstream b2WheelJoint_SetLimits).
func (w *World) SetWheelJointLimits(jointID JointID, lower, upper float64) {
	assert(lower <= upper)

	j := w.getJointSimCheckType(jointID, WheelJoint)
	if lower != j.wheelJoint.lowerTranslation || upper != j.wheelJoint.upperTranslation {
		j.wheelJoint.lowerTranslation = minFloat(lower, upper)
		j.wheelJoint.upperTranslation = maxFloat(lower, upper)
		j.wheelJoint.lowerImpulse = 0.0
		j.wheelJoint.upperImpulse = 0.0
	}
}

// EnableWheelJointMotor enables/disables the wheel joint motor
// (upstream b2WheelJoint_EnableMotor).
func (w *World) EnableWheelJointMotor(jointID JointID, enableMotor bool) {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	if j.wheelJoint.enableMotor != enableMotor {
		j.wheelJoint.motorImpulse = 0.0
		j.wheelJoint.enableMotor = enableMotor
	}
}

// IsWheelJointMotorEnabled reports whether the wheel joint motor is enabled
// (upstream b2WheelJoint_IsMotorEnabled).
func (w *World) IsWheelJointMotorEnabled(jointID JointID) bool {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return j.wheelJoint.enableMotor
}

// SetWheelJointMotorSpeed sets the wheel joint motor speed in radians per
// second (upstream b2WheelJoint_SetMotorSpeed).
func (w *World) SetWheelJointMotorSpeed(jointID JointID, motorSpeed float64) {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	j.wheelJoint.motorSpeed = motorSpeed
}

// WheelJointMotorSpeed returns the wheel joint motor speed in radians per
// second (upstream b2WheelJoint_GetMotorSpeed).
func (w *World) WheelJointMotorSpeed(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return j.wheelJoint.motorSpeed
}

// WheelJointMotorTorque returns the current wheel joint motor torque, usually
// in Newton * meters (upstream b2WheelJoint_GetMotorTorque).
func (w *World) WheelJointMotorTorque(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return w.invH * j.wheelJoint.motorImpulse
}

// SetWheelJointMaxMotorTorque sets the wheel joint maximum motor torque,
// usually in Newton * meters (upstream b2WheelJoint_SetMaxMotorTorque).
func (w *World) SetWheelJointMaxMotorTorque(jointID JointID, torque float64) {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	j.wheelJoint.maxMotorTorque = torque
}

// WheelJointMaxMotorTorque returns the wheel joint maximum motor torque
// (upstream b2WheelJoint_GetMaxMotorTorque).
func (w *World) WheelJointMaxMotorTorque(jointID JointID) float64 {
	j := w.getJointSimCheckType(jointID, WheelJoint)
	return j.wheelJoint.maxMotorTorque
}

// getWheelJointForce returns the wheel joint constraint force
// (upstream b2GetWheelJointForce).
func getWheelJointForce(w *World, base *jointSim) Vec2 {
	idA := base.bodyIDA
	transformA := w.getBodyTransform(idA)

	localAxisA := RotateVector(base.localFrameA.Q, Vec2{X: 1.0, Y: 0.0})
	axisA := RotateVector(transformA.Q, localAxisA)
	perpA := LeftPerp(axisA)

	j := &base.wheelJoint

	perpForce := w.invH * j.perpImpulse
	axialForce := w.invH * (j.springImpulse + j.lowerImpulse - j.upperImpulse)

	force := Add(MulSV(perpForce, perpA), MulSV(axialForce, axisA))
	return force
}

// getWheelJointTorque returns the wheel joint constraint torque
// (upstream b2GetWheelJointTorque).
func getWheelJointTorque(w *World, base *jointSim) float64 {
	return w.invH * base.wheelJoint.motorImpulse
}

// Linear constraint (point-to-line)
// d = pB - pA = xB + rB - xA - rA
// C = dot(ay, d)
// Cdot = dot(d, cross(wA, ay)) + dot(ay, vB + cross(wB, rB) - vA - cross(wA, rA))
//      = -dot(ay, vA) - dot(cross(d + rA, ay), wA) + dot(ay, vB) + dot(cross(rB, ay), vB)
// J = [-ay, -cross(d + rA, ay), ay, cross(rB, ay)]

// Spring linear constraint
// C = dot(ax, d)
// Cdot = = -dot(ax, vA) - dot(cross(d + rA, ax), wA) + dot(ax, vB) + dot(cross(rB, ax), vB)
// J = [-ax -cross(d+rA, ax) ax cross(rB, ax)]

// Motor rotational constraint
// Cdot = wB - wA
// J = [0 0 -1 0 0 1]

// prepareWheelJoint mirrors b2PrepareWheelJoint.
func prepareWheelJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == WheelJoint)

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

	j := &base.wheelJoint

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

	rA := j.frameA.P
	rB := j.frameB.P

	d := Add(j.deltaCenter, Sub(rB, rA))
	axisA := RotateVector(j.frameA.Q, Vec2{X: 1.0, Y: 0.0})
	perpA := LeftPerp(axisA)

	// perpendicular constraint (keep wheel on line)
	s1 := Cross(Add(d, rA), perpA)
	s2 := Cross(rB, perpA)

	// kp = mA + mB + iA * s1 * s1 + iB * s2 * s2
	kp := mA + mB + float64(iA*s1*s1) + float64(iB*s2*s2)
	j.perpMass = 0.0
	if kp > 0.0 {
		j.perpMass = 1.0 / kp
	}

	// spring constraint
	a1 := Cross(Add(d, rA), axisA)
	a2 := Cross(rB, axisA)

	// ka = mA + mB + iA * a1 * a1 + iB * a2 * a2
	ka := mA + mB + float64(iA*a1*a1) + float64(iB*a2*a2)
	j.axialMass = 0.0
	if ka > 0.0 {
		j.axialMass = 1.0 / ka
	}

	j.springSoftness = makeSoft(j.hertz, j.dampingRatio, ctx.h)

	km := iA + iB
	j.motorMass = 0.0
	if km > 0.0 {
		j.motorMass = 1.0 / km
	}

	if !ctx.enableWarmStarting {
		j.perpImpulse = 0.0
		j.springImpulse = 0.0
		j.motorImpulse = 0.0
		j.lowerImpulse = 0.0
		j.upperImpulse = 0.0
	}
}

// warmStartWheelJoint mirrors b2WarmStartWheelJoint.
func warmStartWheelJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == WheelJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.wheelJoint

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
	perpA := LeftPerp(axisA)

	a1 := Cross(Add(d, rA), axisA)
	a2 := Cross(rB, axisA)
	s1 := Cross(Add(d, rA), perpA)
	s2 := Cross(rB, perpA)

	axialImpulse := j.springImpulse + j.lowerImpulse - j.upperImpulse

	p := Add(MulSV(axialImpulse, axisA), MulSV(j.perpImpulse, perpA))
	// LA = axialImpulse * a1 + perpImpulse * s1 + motorImpulse
	la := dot2(axialImpulse, a1, j.perpImpulse, s1) + j.motorImpulse
	// LB = axialImpulse * a2 + perpImpulse * s2 + motorImpulse
	lb := dot2(axialImpulse, a2, j.perpImpulse, s2) + j.motorImpulse

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

// solveWheelJoint mirrors b2SolveWheelJoint.
func solveWheelJoint(base *jointSim, ctx *stepContext, useBias bool) {
	assert(base.jointType == WheelJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.wheelJoint

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

	fixedRotation := iA+iB == 0.0

	// current anchors
	rA := RotateVector(stateA.deltaRotation, j.frameA.P)
	rB := RotateVector(stateB.deltaRotation, j.frameB.P)

	d := Add(Add(Sub(stateB.deltaPosition, stateA.deltaPosition), j.deltaCenter), Sub(rB, rA))
	axisA := RotateVector(j.frameA.Q, Vec2{X: 1.0, Y: 0.0})
	axisA = RotateVector(stateA.deltaRotation, axisA)
	translation := Dot(axisA, d)

	a1 := Cross(Add(d, rA), axisA)
	a2 := Cross(rB, axisA)

	// motor constraint
	if j.enableMotor && !fixedRotation {
		cDot := wB - wA - j.motorSpeed
		impulse := -j.motorMass * cDot
		oldImpulse := j.motorImpulse
		maxImpulse := ctx.h * j.maxMotorTorque
		j.motorImpulse = clampFloat(j.motorImpulse+impulse, -maxImpulse, maxImpulse)
		impulse = j.motorImpulse - oldImpulse

		wA -= float64(iA * impulse)
		wB += float64(iB * impulse)
	}

	// spring constraint
	if j.enableSpring {
		// This is a real spring and should be applied even during relax
		c := translation
		bias := j.springSoftness.biasRate * c
		massScale := j.springSoftness.massScale
		impulseScale := j.springSoftness.impulseScale

		// Cdot = dot(axisA, vB - vA) + a2 * wB - a1 * wA
		cDot := Dot(axisA, Sub(vB, vA)) + float64(a2*wB) - float64(a1*wA)
		// impulse = -massScale * axialMass * (Cdot + bias) - impulseScale * springImpulse
		impulse := cross2(-massScale*j.axialMass, cDot+bias, impulseScale, j.springImpulse)
		j.springImpulse += impulse

		p := MulSV(impulse, axisA)
		la := impulse * a1
		lb := impulse * a2

		vA = MulSub(vA, mA, p)
		wA -= float64(iA * la)
		vB = MulAdd(vB, mB, p)
		wB += float64(iB * lb)
	}

	if j.enableLimit {
		// Lower limit
		{
			c := translation - j.lowerTranslation
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

			// Cdot = dot(axisA, vB - vA) + a2 * wB - a1 * wA
			cDot := Dot(axisA, Sub(vB, vA)) + float64(a2*wB) - float64(a1*wA)
			// impulse = -massScale * axialMass * (Cdot + bias) - impulseScale * lowerImpulse
			impulse := cross2(-massScale*j.axialMass, cDot+bias, impulseScale, j.lowerImpulse)
			oldImpulse := j.lowerImpulse
			j.lowerImpulse = maxFloat(oldImpulse+impulse, 0.0)
			impulse = j.lowerImpulse - oldImpulse

			p := MulSV(impulse, axisA)
			la := impulse * a1
			lb := impulse * a2

			vA = MulSub(vA, mA, p)
			wA -= float64(iA * la)
			vB = MulAdd(vB, mB, p)
			wB += float64(iB * lb)
		}

		// Upper limit
		// Note: signs are flipped to keep C positive when the constraint is
		// satisfied. This also keeps the impulse positive when the limit is
		// active.
		{
			// sign flipped
			c := j.upperTranslation - translation
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
			// Cdot = dot(axisA, vA - vB) + a1 * wA - a2 * wB
			cDot := Dot(axisA, Sub(vA, vB)) + float64(a1*wA) - float64(a2*wB)
			// impulse = -massScale * axialMass * (Cdot + bias) - impulseScale * upperImpulse
			impulse := cross2(-massScale*j.axialMass, cDot+bias, impulseScale, j.upperImpulse)
			oldImpulse := j.upperImpulse
			j.upperImpulse = maxFloat(oldImpulse+impulse, 0.0)
			impulse = j.upperImpulse - oldImpulse

			p := MulSV(impulse, axisA)
			la := impulse * a1
			lb := impulse * a2

			// sign flipped on applied impulse
			vA = MulAdd(vA, mA, p)
			wA += float64(iA * la)
			vB = MulSub(vB, mB, p)
			wB -= float64(iB * lb)
		}
	}

	// point to line constraint
	{
		perpA := LeftPerp(axisA)

		bias := 0.0
		massScale := 1.0
		impulseScale := 0.0
		if useBias {
			c := Dot(perpA, d)
			bias = base.constraintSoftness.biasRate * c
			massScale = base.constraintSoftness.massScale
			impulseScale = base.constraintSoftness.impulseScale
		}

		s1 := Cross(Add(d, rA), perpA)
		s2 := Cross(rB, perpA)
		// Cdot = dot(perpA, vB - vA) + s2 * wB - s1 * wA
		cDot := Dot(perpA, Sub(vB, vA)) + float64(s2*wB) - float64(s1*wA)

		// impulse = -massScale * perpMass * (Cdot + bias) - impulseScale * perpImpulse
		impulse := cross2(-massScale*j.perpMass, cDot+bias, impulseScale, j.perpImpulse)
		j.perpImpulse += impulse

		p := MulSV(impulse, perpA)
		la := impulse * s1
		lb := impulse * s2

		vA = MulSub(vA, mA, p)
		wA -= float64(iA * la)
		vB = MulAdd(vB, mB, p)
		wB += float64(iB * lb)
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

// drawWheelJoint renders a wheel joint (upstream b2DrawWheelJoint).
func drawWheelJoint(draw *DebugDraw, base *jointSim, transformA, transformB Transform, drawScale float64) {
	assert(base.jointType == WheelJoint)

	j := &base.wheelJoint

	frameA := MulTransforms(transformA, base.localFrameA)
	frameB := MulTransforms(transformB, base.localFrameB)
	axisA := RotateVector(frameA.Q, Vec2{X: 1.0, Y: 0.0})

	c1 := ColorGray
	c2 := ColorGreen
	c3 := ColorRed
	c4 := ColorDimGray
	c5 := ColorBlue

	draw.DrawLineFcn(frameA.P, frameB.P, c5, draw.Context)

	if j.enableLimit {
		lower := MulAdd(frameA.P, j.lowerTranslation, axisA)
		upper := MulAdd(frameA.P, j.upperTranslation, axisA)
		perp := LeftPerp(axisA)

		draw.DrawLineFcn(lower, upper, c1, draw.Context)
		draw.DrawLineFcn(MulSub(lower, 0.1*drawScale, perp), MulAdd(lower, 0.1*drawScale, perp), c2, draw.Context)
		draw.DrawLineFcn(MulSub(upper, 0.1*drawScale, perp), MulAdd(upper, 0.1*drawScale, perp), c3, draw.Context)
	} else {
		draw.DrawLineFcn(MulSub(frameA.P, 1.0, axisA), MulAdd(frameA.P, 1.0, axisA), c1, draw.Context)
	}

	draw.DrawPointFcn(frameA.P, 5.0, c1, draw.Context)
	draw.DrawPointFcn(frameB.P, 5.0, c4, draw.Context)
}
