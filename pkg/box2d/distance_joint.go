// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/distance_joint.c
// (plus the b2DistanceJoint struct from src/joint.h).
//
// Public API mapping: b2DistanceJoint_SetLength →
// (*World).SetDistanceJointLength, b2DistanceJoint_GetLength →
// (*World).DistanceJointLength, and so on (getters are Get-less).
//

package box2d

// distanceJoint is the distance joint solver state (upstream b2DistanceJoint
// in joint.h). Lives in the jointSim per-type union.
type distanceJoint struct {
	length           float64
	hertz            float64
	dampingRatio     float64
	lowerSpringForce float64
	upperSpringForce float64
	minLength        float64
	maxLength        float64

	maxMotorForce float64
	motorSpeed    float64

	impulse      float64
	lowerImpulse float64
	upperImpulse float64
	motorImpulse float64

	indexA           int
	indexB           int
	anchorA          Vec2
	anchorB          Vec2
	deltaCenter      Vec2
	distanceSoftness softness
	axialMass        float64

	enableSpring bool
	enableLimit  bool
	enableMotor  bool
}

// SetDistanceJointLength sets the rest length of a distance joint
// (upstream b2DistanceJoint_SetLength).
func (w *World) SetDistanceJointLength(jointID JointID, length float64) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	j := &base.distanceJoint

	j.length = clampFloat(length, LinearSlop, Huge)
	j.impulse = 0.0
	j.lowerImpulse = 0.0
	j.upperImpulse = 0.0
}

// DistanceJointLength returns the rest length of a distance joint
// (upstream b2DistanceJoint_GetLength).
func (w *World) DistanceJointLength(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.length
}

// EnableDistanceJointLimit enables/disables the distance joint limit
// (upstream b2DistanceJoint_EnableLimit).
func (w *World) EnableDistanceJointLimit(jointID JointID, enableLimit bool) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	base.distanceJoint.enableLimit = enableLimit
}

// IsDistanceJointLimitEnabled reports whether the distance joint limit is
// enabled (upstream b2DistanceJoint_IsLimitEnabled).
func (w *World) IsDistanceJointLimitEnabled(jointID JointID) bool {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.enableLimit
}

// SetDistanceJointLengthRange sets the minimum and maximum length parameters
// of a distance joint (upstream b2DistanceJoint_SetLengthRange).
func (w *World) SetDistanceJointLengthRange(jointID JointID, minLength, maxLength float64) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	j := &base.distanceJoint

	minLength = clampFloat(minLength, LinearSlop, Huge)
	maxLength = clampFloat(maxLength, LinearSlop, Huge)
	j.minLength = minFloat(minLength, maxLength)
	j.maxLength = maxFloat(minLength, maxLength)
	j.impulse = 0.0
	j.lowerImpulse = 0.0
	j.upperImpulse = 0.0
}

// DistanceJointMinLength returns the minimum distance joint length
// (upstream b2DistanceJoint_GetMinLength).
func (w *World) DistanceJointMinLength(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.minLength
}

// DistanceJointMaxLength returns the maximum distance joint length
// (upstream b2DistanceJoint_GetMaxLength).
func (w *World) DistanceJointMaxLength(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.maxLength
}

// DistanceJointCurrentLength returns the current length of a distance joint
// (upstream b2DistanceJoint_GetCurrentLength).
func (w *World) DistanceJointCurrentLength(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, DistanceJoint)

	assert(!w.locked)
	if w.locked {
		return 0.0
	}

	transformA := w.getBodyTransform(base.bodyIDA)
	transformB := w.getBodyTransform(base.bodyIDB)

	pA := TransformPoint(transformA, base.localFrameA.P)
	pB := TransformPoint(transformB, base.localFrameB.P)
	d := Sub(pB, pA)
	return Length(d)
}

// EnableDistanceJointSpring enables/disables the distance joint spring. When
// disabled the distance joint is rigid (upstream b2DistanceJoint_EnableSpring).
func (w *World) EnableDistanceJointSpring(jointID JointID, enableSpring bool) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	base.distanceJoint.enableSpring = enableSpring
}

// IsDistanceJointSpringEnabled reports whether the distance joint spring is
// enabled (upstream b2DistanceJoint_IsSpringEnabled).
func (w *World) IsDistanceJointSpringEnabled(jointID JointID) bool {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.enableSpring
}

// SetDistanceJointSpringForceRange sets the force range for the spring
// (upstream b2DistanceJoint_SetSpringForceRange).
func (w *World) SetDistanceJointSpringForceRange(jointID JointID, lowerForce, upperForce float64) {
	assert(lowerForce <= upperForce)
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	base.distanceJoint.lowerSpringForce = lowerForce
	base.distanceJoint.upperSpringForce = upperForce
}

// DistanceJointSpringForceRange returns the force range for the spring as
// (lowerForce, upperForce) (upstream b2DistanceJoint_GetSpringForceRange).
func (w *World) DistanceJointSpringForceRange(jointID JointID) (float64, float64) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.lowerSpringForce, base.distanceJoint.upperSpringForce
}

// SetDistanceJointSpringHertz sets the spring stiffness in Hertz
// (upstream b2DistanceJoint_SetSpringHertz).
func (w *World) SetDistanceJointSpringHertz(jointID JointID, hertz float64) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	base.distanceJoint.hertz = hertz
}

// SetDistanceJointSpringDampingRatio sets the spring damping ratio,
// non-dimensional (upstream b2DistanceJoint_SetSpringDampingRatio).
func (w *World) SetDistanceJointSpringDampingRatio(jointID JointID, dampingRatio float64) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	base.distanceJoint.dampingRatio = dampingRatio
}

// DistanceJointSpringHertz returns the spring Hertz
// (upstream b2DistanceJoint_GetSpringHertz).
func (w *World) DistanceJointSpringHertz(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.hertz
}

// DistanceJointSpringDampingRatio returns the spring damping ratio
// (upstream b2DistanceJoint_GetSpringDampingRatio).
func (w *World) DistanceJointSpringDampingRatio(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.dampingRatio
}

// EnableDistanceJointMotor enables/disables the distance joint motor
// (upstream b2DistanceJoint_EnableMotor).
func (w *World) EnableDistanceJointMotor(jointID JointID, enableMotor bool) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	if enableMotor != base.distanceJoint.enableMotor {
		base.distanceJoint.enableMotor = enableMotor
		base.distanceJoint.motorImpulse = 0.0
	}
}

// IsDistanceJointMotorEnabled reports whether the distance joint motor is
// enabled (upstream b2DistanceJoint_IsMotorEnabled).
func (w *World) IsDistanceJointMotorEnabled(jointID JointID) bool {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.enableMotor
}

// SetDistanceJointMotorSpeed sets the distance joint motor speed, usually in
// meters per second (upstream b2DistanceJoint_SetMotorSpeed).
func (w *World) SetDistanceJointMotorSpeed(jointID JointID, motorSpeed float64) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	base.distanceJoint.motorSpeed = motorSpeed
}

// DistanceJointMotorSpeed returns the distance joint motor speed
// (upstream b2DistanceJoint_GetMotorSpeed).
func (w *World) DistanceJointMotorSpeed(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.motorSpeed
}

// DistanceJointMotorForce returns the current motor force, usually in
// Newtons (upstream b2DistanceJoint_GetMotorForce).
func (w *World) DistanceJointMotorForce(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return w.invH * base.distanceJoint.motorImpulse
}

// SetDistanceJointMaxMotorForce sets the maximum motor force, usually in
// Newtons (upstream b2DistanceJoint_SetMaxMotorForce).
func (w *World) SetDistanceJointMaxMotorForce(jointID JointID, force float64) {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	base.distanceJoint.maxMotorForce = force
}

// DistanceJointMaxMotorForce returns the maximum motor force, usually in
// Newtons (upstream b2DistanceJoint_GetMaxMotorForce).
func (w *World) DistanceJointMaxMotorForce(jointID JointID) float64 {
	base := w.getJointSimCheckType(jointID, DistanceJoint)
	return base.distanceJoint.maxMotorForce
}

// getDistanceJointForce returns the distance joint constraint force
// (upstream b2GetDistanceJointForce).
func getDistanceJointForce(w *World, base *jointSim) Vec2 {
	j := &base.distanceJoint

	transformA := w.getBodyTransform(base.bodyIDA)
	transformB := w.getBodyTransform(base.bodyIDB)

	pA := TransformPoint(transformA, base.localFrameA.P)
	pB := TransformPoint(transformB, base.localFrameB.P)
	d := Sub(pB, pA)
	axis := Normalize(d)
	force := (j.impulse + j.lowerImpulse - j.upperImpulse + j.motorImpulse) * w.invH
	return MulSV(force, axis)
}

// 1-D constrained system
// m (v2 - v1) = lambda
// v2 + (beta/h) * x1 + gamma * lambda = 0, gamma has units of inverse mass.
// x2 = x1 + h * v2

// 1-D mass-damper-spring system
// m (v2 - v1) + h * d * v2 + h * k *

// C = norm(p2 - p1) - L
// u = (p2 - p1) / norm(p2 - p1)
// Cdot = dot(u, v2 + cross(w2, r2) - v1 - cross(w1, r1))
// J = [-u -cross(r1, u) u cross(r2, u)]
// K = J * invM * JT
//   = invMass1 + invI1 * cross(r1, u)^2 + invMass2 + invI2 * cross(r2, u)^2

// prepareDistanceJoint mirrors b2PrepareDistanceJoint.
func prepareDistanceJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == DistanceJoint)

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

	j := &base.distanceJoint

	j.indexA = NullIndex
	if bodyA.setIndex == awakeSet {
		j.indexA = localIndexA
	}
	j.indexB = NullIndex
	if bodyB.setIndex == awakeSet {
		j.indexB = localIndexB
	}

	// initial anchors in world space
	j.anchorA = RotateVector(bodySimA.transform.Q, Sub(base.localFrameA.P, bodySimA.localCenter))
	j.anchorB = RotateVector(bodySimB.transform.Q, Sub(base.localFrameB.P, bodySimB.localCenter))
	j.deltaCenter = Sub(bodySimB.center, bodySimA.center)

	rA := j.anchorA
	rB := j.anchorB
	separation := Add(Sub(rB, rA), j.deltaCenter)
	axis := Normalize(separation)

	// compute effective mass
	crA := Cross(rA, axis)
	crB := Cross(rB, axis)
	// k = mA + mB + iA * crA * crA + iB * crB * crB
	k := mA + mB + float64(iA*crA*crA) + float64(iB*crB*crB)
	j.axialMass = 0.0
	if k > 0.0 {
		j.axialMass = 1.0 / k
	}

	j.distanceSoftness = makeSoft(j.hertz, j.dampingRatio, ctx.h)

	if !ctx.enableWarmStarting {
		j.impulse = 0.0
		j.lowerImpulse = 0.0
		j.upperImpulse = 0.0
		j.motorImpulse = 0.0
	}
}

// warmStartDistanceJoint mirrors b2WarmStartDistanceJoint.
func warmStartDistanceJoint(base *jointSim, ctx *stepContext) {
	assert(base.jointType == DistanceJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.distanceJoint
	stateA := &dummyState
	if j.indexA != NullIndex {
		stateA = &ctx.states[j.indexA]
	}
	stateB := &dummyState
	if j.indexB != NullIndex {
		stateB = &ctx.states[j.indexB]
	}

	rA := RotateVector(stateA.deltaRotation, j.anchorA)
	rB := RotateVector(stateB.deltaRotation, j.anchorB)

	ds := Add(Sub(stateB.deltaPosition, stateA.deltaPosition), Sub(rB, rA))
	separation := Add(j.deltaCenter, ds)
	axis := Normalize(separation)

	axialImpulse := j.impulse + j.lowerImpulse - j.upperImpulse + j.motorImpulse
	p := MulSV(axialImpulse, axis)

	if stateA.flags&dynamicFlag != 0 {
		stateA.linearVelocity = MulSub(stateA.linearVelocity, mA, p)
		// wA -= iA * cross(rA, P)
		stateA.angularVelocity -= float64(iA * Cross(rA, p))
	}

	if stateB.flags&dynamicFlag != 0 {
		stateB.linearVelocity = MulAdd(stateB.linearVelocity, mB, p)
		// wB += iB * cross(rB, P)
		stateB.angularVelocity += float64(iB * Cross(rB, p))
	}
}

// solveDistanceJoint mirrors b2SolveDistanceJoint.
func solveDistanceJoint(base *jointSim, ctx *stepContext, useBias bool) {
	assert(base.jointType == DistanceJoint)

	mA := base.invMassA
	mB := base.invMassB
	iA := base.invIA
	iB := base.invIB

	// dummy state for static bodies
	dummyState := identityBodyState

	j := &base.distanceJoint
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

	// current anchors
	rA := RotateVector(stateA.deltaRotation, j.anchorA)
	rB := RotateVector(stateB.deltaRotation, j.anchorB)

	// current separation
	ds := Add(Sub(stateB.deltaPosition, stateA.deltaPosition), Sub(rB, rA))
	separation := Add(j.deltaCenter, ds)

	length := Length(separation)
	axis := Normalize(separation)

	// joint is soft if
	// - spring is enabled
	// - and (joint limit is disabled or limits are not equal)
	if j.enableSpring && (j.minLength < j.maxLength || !j.enableLimit) {
		// spring
		if j.hertz > 0.0 {
			// Cdot = dot(u, v + cross(w, r))
			vr := Add(Sub(vB, vA), Sub(CrossSV(wB, rB), CrossSV(wA, rA)))
			cDot := Dot(axis, vr)
			c := length - j.length
			bias := j.distanceSoftness.biasRate * c

			m := j.distanceSoftness.massScale * j.axialMass
			oldImpulse := j.impulse
			// impulse = -m * (Cdot + bias) - impulseScale * oldImpulse
			impulse := cross2(-m, cDot+bias, j.distanceSoftness.impulseScale, oldImpulse)

			h := ctx.h
			j.impulse = clampFloat(j.impulse+impulse, j.lowerSpringForce*h, j.upperSpringForce*h)
			impulse = j.impulse - oldImpulse

			p := MulSV(impulse, axis)
			vA = MulSub(vA, mA, p)
			// wA -= iA * cross(rA, P)
			wA -= float64(iA * Cross(rA, p))
			vB = MulAdd(vB, mB, p)
			// wB += iB * cross(rB, P)
			wB += float64(iB * Cross(rB, p))
		}

		if j.enableMotor {
			vr := Add(Sub(vB, vA), Sub(CrossSV(wB, rB), CrossSV(wA, rA)))
			cDot := Dot(axis, vr)
			impulse := j.axialMass * (j.motorSpeed - cDot)
			oldImpulse := j.motorImpulse
			maxImpulse := ctx.h * j.maxMotorForce
			j.motorImpulse = clampFloat(j.motorImpulse+impulse, -maxImpulse, maxImpulse)
			impulse = j.motorImpulse - oldImpulse

			p := MulSV(impulse, axis)
			vA = MulSub(vA, mA, p)
			wA -= float64(iA * Cross(rA, p))
			vB = MulAdd(vB, mB, p)
			wB += float64(iB * Cross(rB, p))
		}

		if j.enableLimit {
			// lower limit
			{
				vr := Add(Sub(vB, vA), Sub(CrossSV(wB, rB), CrossSV(wA, rA)))
				cDot := Dot(axis, vr)

				c := length - j.minLength

				bias := 0.0
				massCoeff := 1.0
				impulseCoeff := 0.0
				if c > 0.0 {
					// speculative
					bias = c * ctx.invH
				} else if useBias {
					bias = base.constraintSoftness.biasRate * c
					massCoeff = base.constraintSoftness.massScale
					impulseCoeff = base.constraintSoftness.impulseScale
				}

				// impulse = -massCoeff * axialMass * (Cdot + bias) - impulseCoeff * lowerImpulse
				impulse := cross2(-massCoeff*j.axialMass, cDot+bias, impulseCoeff, j.lowerImpulse)
				newImpulse := maxFloat(0.0, j.lowerImpulse+impulse)
				impulse = newImpulse - j.lowerImpulse
				j.lowerImpulse = newImpulse

				p := MulSV(impulse, axis)
				vA = MulSub(vA, mA, p)
				wA -= float64(iA * Cross(rA, p))
				vB = MulAdd(vB, mB, p)
				wB += float64(iB * Cross(rB, p))
			}

			// upper
			{
				vr := Add(Sub(vA, vB), Sub(CrossSV(wA, rA), CrossSV(wB, rB)))
				cDot := Dot(axis, vr)

				c := j.maxLength - length

				bias := 0.0
				massScale := 1.0
				impulseScale := 0.0
				if c > 0.0 {
					// speculative
					bias = c * ctx.invH
				} else if useBias {
					bias = base.constraintSoftness.biasRate * c
					massScale = base.constraintSoftness.massScale
					impulseScale = base.constraintSoftness.impulseScale
				}

				// impulse = -massScale * axialMass * (Cdot + bias) - impulseScale * upperImpulse
				impulse := cross2(-massScale*j.axialMass, cDot+bias, impulseScale, j.upperImpulse)
				newImpulse := maxFloat(0.0, j.upperImpulse+impulse)
				impulse = newImpulse - j.upperImpulse
				j.upperImpulse = newImpulse

				p := MulSV(-impulse, axis)
				vA = MulSub(vA, mA, p)
				wA -= float64(iA * Cross(rA, p))
				vB = MulAdd(vB, mB, p)
				wB += float64(iB * Cross(rB, p))
			}
		}
	} else {
		// rigid constraint
		vr := Add(Sub(vB, vA), Sub(CrossSV(wB, rB), CrossSV(wA, rA)))
		cDot := Dot(axis, vr)

		c := length - j.length

		bias := 0.0
		massScale := 1.0
		impulseScale := 0.0
		if useBias {
			bias = base.constraintSoftness.biasRate * c
			massScale = base.constraintSoftness.massScale
			impulseScale = base.constraintSoftness.impulseScale
		}

		// impulse = -massScale * axialMass * (Cdot + bias) - impulseScale * impulse
		impulse := cross2(-massScale*j.axialMass, cDot+bias, impulseScale, j.impulse)
		j.impulse += impulse

		p := MulSV(impulse, axis)
		vA = MulSub(vA, mA, p)
		wA -= float64(iA * Cross(rA, p))
		vB = MulAdd(vB, mB, p)
		wB += float64(iB * Cross(rB, p))
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

// drawDistanceJoint renders a distance joint (upstream b2DrawDistanceJoint).
func drawDistanceJoint(draw *DebugDraw, base *jointSim, transformA, transformB Transform) {
	assert(base.jointType == DistanceJoint)

	j := &base.distanceJoint

	pA := TransformPoint(transformA, base.localFrameA.P)
	pB := TransformPoint(transformB, base.localFrameB.P)

	axis := Normalize(Sub(pB, pA))

	if j.minLength < j.maxLength && j.enableLimit {
		pMin := MulAdd(pA, j.minLength, axis)
		pMax := MulAdd(pA, j.maxLength, axis)
		offset := MulSV(0.05*GetLengthUnitsPerMeter(), RightPerp(axis))

		if j.minLength > LinearSlop {
			draw.DrawLineFcn(Sub(pMin, offset), Add(pMin, offset), ColorLightGreen, draw.Context)
		}

		if j.maxLength < Huge {
			draw.DrawLineFcn(Sub(pMax, offset), Add(pMax, offset), ColorRed, draw.Context)
		}

		if j.minLength > LinearSlop && j.maxLength < Huge {
			draw.DrawLineFcn(pMin, pMax, ColorGray, draw.Context)
		}
	}

	draw.DrawLineFcn(pA, pB, ColorWhite, draw.Context)
	draw.DrawPointFcn(pA, 4.0, ColorWhite, draw.Context)
	draw.DrawPointFcn(pB, 4.0, ColorWhite, draw.Context)

	if j.hertz > 0.0 && j.enableSpring {
		pRest := MulAdd(pA, j.length, axis)
		draw.DrawPointFcn(pRest, 4.0, ColorBlue, draw.Context)
	}
}
