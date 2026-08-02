// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/contact_solver.h, src/contact_solver.c
// (scalar path only).
//
// DESIGN DEVIATION (approved): ALL contacts — every graph color AND the
// overflow color — are solved through the scalar routines in this file, which
// transliterate the upstream b2*OverflowContacts functions. The upstream SIMD
// batch path (b2FloatW, b2ContactConstraintSIMD, b2PrepareContactsTask and
// friends) is NOT ported: per-lane arithmetic of the upstream B2_SIMD_NONE
// build is identical to the scalar path contact-by-contact, and this port is
// single-threaded. Each routine takes a colorIndex so the solver can run the
// same code over graph colors and the overflow set. Iteration order is fixed
// by the solve loop in solver.go: within each stage the overflow color runs
// first (upstream: overflow constraints have lower priority and are executed
// before the per-color stages by the main thread), then the active colors in
// ascending color-index order; contacts within a color run in localIndex
// order. The graph coloring itself is kept (built in constraint_graph.go)
// because it fixes the solve order and enables future parallelism.
//
// Other deviations from upstream:
//   - world.enableContactSoftening (experimental, default off) has no effect:
//     upstream only implements it in the SIMD prepare path
//     (b2PrepareContactsTask); the scalar overflow prepare path ported here
//     never had it.
//
// contact separation for sub-stepping (upstream comment):
//	s = s0 + dot(cB + rB - cA - rA, normal)
//	normal is held constant
//	body positions c can translate and anchors r can rotate
//	s(t) = s0 + dot(cB(t) + rB(t) - cA(t) - rA(t), normal)
//	s(t) = s0 + dot(cB0 + dpB + rot(dqB, rB0) - cA0 - dpA - rot(dqA, rA0), normal)
//	s(t) = s0 + dot(cB0 - cA0, normal) + dot(dpB - dpA + rot(dqB, rB0) - rot(dqA, rA0), normal)
//	s_base = s0 + dot(cB0 - cA0, normal)

package box2d

// contactConstraintPoint is one scalar contact constraint point (upstream
// b2ContactConstraintPoint).
type contactConstraintPoint struct {
	anchorA, anchorB   Vec2
	baseSeparation     float64
	relativeVelocity   float64
	normalImpulse      float64
	tangentImpulse     float64
	totalNormalImpulse float64
	normalMass         float64
	tangentMass        float64
}

// contactConstraint is one scalar contact constraint (upstream
// b2ContactConstraint).
type contactConstraint struct {
	// base-1, 0 for null
	indexA int
	indexB int

	points             [2]contactConstraintPoint
	normal             Vec2
	invMassA, invMassB float64
	invIA, invIB       float64
	friction           float64
	restitution        float64
	tangentSpeed       float64
	rollingResistance  float64
	rollingMass        float64
	rollingImpulse     float64
	softness           softness
	pointCount         int
}

// prepareContactsColor prepares the contact constraints of one graph color
// (upstream b2PrepareOverflowContacts, generalized to any color per the file
// header).
func (w *World) prepareContactsColor(ctx *stepContext, colorIndex int) {
	graph := ctx.graph
	color := &graph.colors[colorIndex]
	contactSims := color.contactSims
	contactCount := len(contactSims)
	// Same length as contactSims (see solver.go); re-slicing lets the compiler
	// drop the per-iteration bounds checks below.
	constraints := color.constraints[:contactCount]
	awakeStates := ctx.states

	// Stiffer for static contacts to avoid bodies getting pushed through the
	// ground.
	contactSoftness := ctx.contactSoftness
	staticSoftness := ctx.staticSoftness

	warmStartScale := 0.0
	if w.enableWarmStarting {
		warmStartScale = 1.0
	}

	for i := range contactCount {
		contactSim := &contactSims[i]

		manifold := &contactSim.manifold
		pointCount := manifold.PointCount

		assert(0 < pointCount && pointCount <= 2)

		indexA := contactSim.bodySimIndexA
		indexB := contactSim.bodySimIndexB

		constraint := &constraints[i]

		// 0 is null
		constraint.indexA = indexA + 1
		constraint.indexB = indexB + 1
		constraint.normal = manifold.Normal
		constraint.friction = contactSim.friction
		constraint.restitution = contactSim.restitution
		constraint.rollingResistance = contactSim.rollingResistance
		constraint.rollingImpulse = warmStartScale * manifold.RollingImpulse
		constraint.tangentSpeed = contactSim.tangentSpeed
		constraint.pointCount = pointCount

		vA := Vec2Zero
		wA := 0.0
		mA := contactSim.invMassA
		iA := contactSim.invIA
		if indexA != NullIndex {
			stateA := &awakeStates[indexA]
			vA = stateA.linearVelocity
			wA = stateA.angularVelocity
		}

		vB := Vec2Zero
		wB := 0.0
		mB := contactSim.invMassB
		iB := contactSim.invIB
		if indexB != NullIndex {
			stateB := &awakeStates[indexB]
			vB = stateB.linearVelocity
			wB = stateB.angularVelocity
		}

		if indexA == NullIndex || indexB == NullIndex {
			constraint.softness = staticSoftness
		} else {
			constraint.softness = contactSoftness
		}

		// copy mass into constraint to avoid cache misses during sub-stepping
		constraint.invMassA = mA
		constraint.invIA = iA
		constraint.invMassB = mB
		constraint.invIB = iB

		{
			k := iA + iB
			if k > 0.0 {
				constraint.rollingMass = 1.0 / k
			} else {
				constraint.rollingMass = 0.0
			}
		}

		normal := constraint.normal
		tangent := RightPerp(constraint.normal)

		// One bounds check each instead of one per iteration below.
		mps := manifold.Points[:pointCount]
		cps := constraint.points[:pointCount]
		for j := range mps {
			mp := &mps[j]
			cp := &cps[j]

			cp.normalImpulse = warmStartScale * mp.NormalImpulse
			cp.tangentImpulse = warmStartScale * mp.TangentImpulse
			cp.totalNormalImpulse = 0.0

			rA := mp.AnchorA
			rB := mp.AnchorB

			cp.anchorA = rA
			cp.anchorB = rB
			cp.baseSeparation = mp.Separation - Dot(Sub(rB, rA), normal)

			rnA := Cross(rA, normal)
			rnB := Cross(rB, normal)
			// kNormal = mA + mB + iA * rnA * rnA + iB * rnB * rnB
			kNormal := mA + mB + float64(iA*rnA*rnA) + float64(iB*rnB*rnB)
			if kNormal > 0.0 {
				cp.normalMass = 1.0 / kNormal
			} else {
				cp.normalMass = 0.0
			}

			rtA := Cross(rA, tangent)
			rtB := Cross(rB, tangent)
			// kTangent = mA + mB + iA * rtA * rtA + iB * rtB * rtB
			kTangent := mA + mB + float64(iA*rtA*rtA) + float64(iB*rtB*rtB)
			if kTangent > 0.0 {
				cp.tangentMass = 1.0 / kTangent
			} else {
				cp.tangentMass = 0.0
			}

			// Save relative velocity for restitution
			vrA := Add(vA, CrossSV(wA, rA))
			vrB := Add(vB, CrossSV(wB, rB))
			cp.relativeVelocity = Dot(normal, Sub(vrB, vrA))
		}
	}
}

// warmStartContactsColor applies the stored impulses of one graph color
// (upstream b2WarmStartOverflowContacts, generalized to any color per the
// file header).
func (w *World) warmStartContactsColor(ctx *stepContext, colorIndex int) {
	graph := ctx.graph
	color := &graph.colors[colorIndex]
	contactCount := len(color.contactSims)
	// Same length as contactSims (see solver.go); re-slicing lets the compiler
	// drop the per-iteration bounds checks below.
	constraints := color.constraints[:contactCount]
	awake := &w.solverSets[awakeSet]
	states := awake.bodyStates

	// This is a dummy state to represent a static body because static bodies
	// don't have a solver body.
	dummyState := identityBodyState

	for i := range contactCount {
		constraint := &constraints[i]

		indexA := constraint.indexA - 1
		indexB := constraint.indexB - 1

		stateA := &dummyState
		if indexA != NullIndex {
			stateA = &states[indexA]
		}
		stateB := &dummyState
		if indexB != NullIndex {
			stateB = &states[indexB]
		}

		vA := stateA.linearVelocity
		wA := stateA.angularVelocity
		vB := stateB.linearVelocity
		wB := stateB.angularVelocity

		mA := constraint.invMassA
		iA := constraint.invIA
		mB := constraint.invMassB
		iB := constraint.invIB

		normal := constraint.normal
		tangent := RightPerp(constraint.normal)
		pointCount := constraint.pointCount

		// One bounds check instead of one per iteration below.
		cps := constraint.points[:pointCount]
		for j := range cps {
			cp := &cps[j]

			// fixed anchors
			rA := cp.anchorA
			rB := cp.anchorB

			p := Add(MulSV(cp.normalImpulse, normal), MulSV(cp.tangentImpulse, tangent))

			cp.totalNormalImpulse += cp.normalImpulse

			// wA -= iA * cross(rA, P)
			wA -= float64(iA * Cross(rA, p))
			vA = MulAdd(vA, -mA, p)
			// wB += iB * cross(rB, P)
			wB += float64(iB * Cross(rB, p))
			vB = MulAdd(vB, mB, p)
		}

		// wA -= iA * rollingImpulse; wB += iB * rollingImpulse
		wA -= float64(iA * constraint.rollingImpulse)
		wB += float64(iB * constraint.rollingImpulse)

		if stateA.flags&dynamicFlag != 0 {
			stateA.linearVelocity = vA
			stateA.angularVelocity = wA
		}

		if stateB.flags&dynamicFlag != 0 {
			stateB.linearVelocity = vB
			stateB.angularVelocity = wB
		}
	}
}

// solveContactsColor solves the contact constraints of one graph color with
// TGS soft constraints (upstream b2SolveOverflowContacts, generalized to any
// color per the file header). useBias selects the solve stage (true) versus
// the relax stage (false).
func (w *World) solveContactsColor(ctx *stepContext, colorIndex int, useBias bool) {
	graph := ctx.graph
	color := &graph.colors[colorIndex]
	contactCount := len(color.contactSims)
	// Same length as contactSims (see solver.go); re-slicing lets the compiler
	// drop the per-iteration bounds checks below.
	constraints := color.constraints[:contactCount]
	awake := &w.solverSets[awakeSet]
	states := awake.bodyStates

	invH := ctx.invH
	contactSpeed := w.contactSpeed

	// This is a dummy body to represent a static body since static bodies
	// don't have a solver body.
	dummyState := identityBodyState

	for i := range contactCount {
		constraint := &constraints[i]
		mA := constraint.invMassA
		iA := constraint.invIA
		mB := constraint.invMassB
		iB := constraint.invIB

		indexA := constraint.indexA - 1
		indexB := constraint.indexB - 1

		stateA := &dummyState
		if indexA != NullIndex {
			stateA = &states[indexA]
		}
		vA := stateA.linearVelocity
		wA := stateA.angularVelocity
		dqA := stateA.deltaRotation

		stateB := &dummyState
		if indexB != NullIndex {
			stateB = &states[indexB]
		}
		vB := stateB.linearVelocity
		wB := stateB.angularVelocity
		dqB := stateB.deltaRotation

		dp := Sub(stateB.deltaPosition, stateA.deltaPosition)

		normal := constraint.normal
		tangent := RightPerp(normal)
		friction := constraint.friction
		soft := constraint.softness

		pointCount := constraint.pointCount
		totalNormalImpulse := 0.0

		// One bounds check instead of one per iteration in the two loops below.
		cps := constraint.points[:pointCount]

		// Non-penetration
		for j := range cps {
			cp := &cps[j]

			// fixed anchor points
			rA := cp.anchorA
			rB := cp.anchorB

			// compute current separation
			// this is subject to round-off error if the anchor is far from the
			// body center of mass
			ds := Add(dp, Sub(RotateVector(dqB, rB), RotateVector(dqA, rA)))
			s := cp.baseSeparation + Dot(ds, normal)

			velocityBias := 0.0
			massScale := 1.0
			impulseScale := 0.0
			switch {
			case s > 0.0:
				// speculative bias
				velocityBias = s * invH
			case useBias:
				// velocityBias = max(massScale * biasRate * s, -contactSpeed)
				velocityBias = maxFloat(soft.massScale*soft.biasRate*s, -contactSpeed)
				massScale = soft.massScale
				impulseScale = soft.impulseScale
			default:
			}

			// relative normal velocity at contact
			vrA := Add(vA, CrossSV(wA, rA))
			vrB := Add(vB, CrossSV(wB, rB))
			vn := Dot(Sub(vrB, vrA), normal)

			// incremental normal impulse
			// impulse = -normalMass * (massScale * vn + velocityBias) - impulseScale * normalImpulse
			impulse := float64(-cp.normalMass*mulAdd(massScale, vn, velocityBias)) - float64(impulseScale*cp.normalImpulse)

			// clamp the accumulated impulse
			newImpulse := maxFloat(cp.normalImpulse+impulse, 0.0)
			impulse = newImpulse - cp.normalImpulse
			cp.normalImpulse = newImpulse
			cp.totalNormalImpulse += impulse

			totalNormalImpulse += newImpulse

			// apply normal impulse
			p := MulSV(impulse, normal)
			vA = MulSub(vA, mA, p)
			wA -= float64(iA * Cross(rA, p))

			vB = MulAdd(vB, mB, p)
			wB += float64(iB * Cross(rB, p))
		}

		// Friction
		for j := range cps {
			cp := &cps[j]

			// fixed anchor points
			rA := cp.anchorA
			rB := cp.anchorB

			// relative tangent velocity at contact
			vrB := Add(vB, CrossSV(wB, rB))
			vrA := Add(vA, CrossSV(wA, rA))

			// vt = dot(vrB - sB * tangent - (vrA + sA * tangent), tangent)
			//    = dot(vrB - vrA, tangent) - (sA + sB)
			vt := Dot(Sub(vrB, vrA), tangent) - constraint.tangentSpeed

			// incremental tangent impulse
			impulse := float64(cp.tangentMass * -vt)

			// clamp the accumulated force
			maxFriction := friction * cp.normalImpulse
			newImpulse := clampFloat(cp.tangentImpulse+impulse, -maxFriction, maxFriction)
			impulse = newImpulse - cp.tangentImpulse
			cp.tangentImpulse = newImpulse

			// apply tangent impulse
			p := MulSV(impulse, tangent)
			vA = MulSub(vA, mA, p)
			wA -= float64(iA * Cross(rA, p))
			vB = MulAdd(vB, mB, p)
			wB += float64(iB * Cross(rB, p))
		}

		// Rolling resistance
		{
			deltaLambda := float64(-constraint.rollingMass * (wB - wA))
			lambda := constraint.rollingImpulse
			maxLambda := constraint.rollingResistance * totalNormalImpulse
			constraint.rollingImpulse = clampFloat(lambda+deltaLambda, -maxLambda, maxLambda)
			deltaLambda = constraint.rollingImpulse - lambda

			wA -= float64(iA * deltaLambda)
			wB += float64(iB * deltaLambda)
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
}

// applyRestitutionColor applies restitution to the contact constraints of one
// graph color (upstream b2ApplyOverflowRestitution, generalized to any color
// per the file header).
func (w *World) applyRestitutionColor(ctx *stepContext, colorIndex int) {
	graph := ctx.graph
	color := &graph.colors[colorIndex]
	contactCount := len(color.contactSims)
	// Same length as contactSims (see solver.go); re-slicing lets the compiler
	// drop the per-iteration bounds checks below.
	constraints := color.constraints[:contactCount]
	awake := &w.solverSets[awakeSet]
	states := awake.bodyStates

	threshold := w.restitutionThreshold

	// dummy state to represent a static body
	dummyState := identityBodyState

	for i := range contactCount {
		constraint := &constraints[i]

		restitution := constraint.restitution
		if restitution == 0.0 {
			continue
		}

		mA := constraint.invMassA
		iA := constraint.invIA
		mB := constraint.invMassB
		iB := constraint.invIB

		indexA := constraint.indexA - 1
		indexB := constraint.indexB - 1

		stateA := &dummyState
		if indexA != NullIndex {
			stateA = &states[indexA]
		}
		vA := stateA.linearVelocity
		wA := stateA.angularVelocity

		stateB := &dummyState
		if indexB != NullIndex {
			stateB = &states[indexB]
		}
		vB := stateB.linearVelocity
		wB := stateB.angularVelocity

		normal := constraint.normal
		pointCount := constraint.pointCount

		// it is possible to get more accurate restitution by iterating
		// this only makes a difference if there are two contact points
		// (one bounds check instead of one per iteration)
		cps := constraint.points[:pointCount]
		for j := range cps {
			cp := &cps[j]

			// if the normal impulse is zero then there was no collision
			// this skips speculative contact points that didn't generate an
			// impulse. The max normal impulse is used in case there was a
			// collision that moved away within the sub-step process.
			if cp.relativeVelocity > -threshold || cp.totalNormalImpulse == 0.0 {
				continue
			}

			// fixed anchor points
			rA := cp.anchorA
			rB := cp.anchorB

			// relative normal velocity at contact
			vrB := Add(vB, CrossSV(wB, rB))
			vrA := Add(vA, CrossSV(wA, rA))
			vn := Dot(Sub(vrB, vrA), normal)

			// compute normal impulse
			// impulse = -normalMass * (vn + restitution * relativeVelocity)
			impulse := float64(-cp.normalMass * (vn + float64(restitution*cp.relativeVelocity)))

			// clamp the accumulated impulse
			newImpulse := maxFloat(cp.normalImpulse+impulse, 0.0)
			impulse = newImpulse - cp.normalImpulse
			cp.normalImpulse = newImpulse
			cp.totalNormalImpulse += impulse

			// apply contact impulse
			p := MulSV(impulse, normal)
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
}

// storeImpulsesColor copies the constraint impulses of one graph color back
// into the contact manifolds for warm starting (upstream
// b2StoreOverflowImpulses, generalized to any color per the file header).
func (w *World) storeImpulsesColor(ctx *stepContext, colorIndex int) {
	graph := ctx.graph
	color := &graph.colors[colorIndex]
	contactSims := color.contactSims
	contactCount := len(contactSims)
	// Same length as contactSims (see solver.go); re-slicing lets the compiler
	// drop the per-iteration bounds checks below.
	constraints := color.constraints[:contactCount]

	for i := range contactCount {
		constraint := &constraints[i]
		contact := &contactSims[i]
		manifold := &contact.manifold
		pointCount := manifold.PointCount

		// One bounds check each instead of one per iteration below.
		mps := manifold.Points[:pointCount]
		cps := constraint.points[:pointCount]
		for j := range mps {
			mp := &mps[j]
			cp := &cps[j]
			mp.NormalImpulse = cp.normalImpulse
			mp.TangentImpulse = cp.tangentImpulse
			mp.TotalNormalImpulse = cp.totalNormalImpulse
			mp.NormalVelocity = cp.relativeVelocity
		}

		manifold.RollingImpulse = constraint.rollingImpulse
	}
}
