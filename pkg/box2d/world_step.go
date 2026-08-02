// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/physics_world.c
// (the step portion: b2CollideTask, b2Collide, b2World_Step and the event
// accessors; world CRUD lives in world.go).
//
// Deviations from upstream (single-threaded port):
//   - b2CollideTask runs as one serial loop over the gathered contact sim
//     pointers (graph colors in ascending order, then the awake non-touching
//     contacts), exactly the order upstream gathers them for the parallel-for.
//   - The per-worker contactStateBitSet lives on World.taskContext; no
//     bitwise-OR merge is needed.
//   - DETERMINISM: the b2Profile timing fields stay zero — upstream fills
//     them with wall-clock timings (b2GetTicks), which this port forbids.
//     Opt-in timing arrives with stage E13.
//   - The sensor pass (b2OverlapSensors) keeps its upstream position: after
//     the solve and before the end-event buffer swap, so sensor end events
//     land in the buffer the next SensorEvents call reads. See sensor.go.

package box2d

// collideTask updates the narrow-phase state of a range of awake contacts
// (upstream b2CollideTask).
func (w *World) collideTask(startIndex, endIndex int, ctx *stepContext) {
	contactSims := ctx.contacts
	tc := &w.taskContext

	assert(startIndex < endIndex)

	recycleDistance := w.contactRecycleDistance
	const speculativeDistance = SpeculativeDistance
	recycleDistanceNonTouching := minFloat(recycleDistance, speculativeDistance)

	for contactIndex := startIndex; contactIndex < endIndex; contactIndex++ {
		contactSim := contactSims[contactIndex]

		contactID := contactSim.contactID

		shapeA := &w.shapes[contactSim.shapeIDA]
		shapeB := &w.shapes[contactSim.shapeIDB]

		// Do proxies still overlap?
		overlap := AABBOverlaps(shapeA.fatAABB, shapeB.fatAABB)
		if !overlap {
			contactSim.simFlags |= simDisjoint
			contactSim.simFlags &^= simTouchingFlag
			setBit(&tc.contactStateBitSet, uint32(contactID))
		} else {
			wasTouching := contactSim.simFlags&simTouchingFlag != 0

			// Update contact respecting shape/body order (A,B)
			bodyA := &w.bodies[shapeA.bodyID]
			bodyB := &w.bodies[shapeB.bodyID]
			bodySimA := w.getBodySim(bodyA)
			bodySimB := w.getBodySim(bodyB)
			transformA := bodySimA.transform
			transformB := bodySimB.transform

			// These may not be skipped by the relative transform check below
			if bodyA.setIndex == awakeSet {
				contactSim.bodySimIndexA = bodyA.localIndex
			} else {
				contactSim.bodySimIndexA = NullIndex
			}
			contactSim.invMassA = bodySimA.invMass
			contactSim.invIA = bodySimA.invInertia

			if bodyB.setIndex == awakeSet {
				contactSim.bodySimIndexB = bodyB.localIndex
			} else {
				contactSim.bodySimIndexB = NullIndex
			}
			contactSim.invMassB = bodySimB.invMass
			contactSim.invIB = bodySimB.invInertia

			// Contact recycling optimization. Please cite this code if you
			// use this optimization. This is inspired by persistent contact
			// manifolds used in some physics engines, such as PhysX. However,
			// this allows larger relative motion and has fewer tuning
			// parameters (just one).
			if recycleDistance > 0.0 && contactSim.simFlags&simRelativeTransformValid != 0 {
				xf := InvMulTransforms(transformA, transformB)
				xfc := InvMulTransforms(contactSim.cachedTransformA, contactSim.cachedTransformB)
				maxExtentA := 0.0
				if bodyA.bodyType != StaticBody {
					maxExtentA = bodySimA.maxExtent
				}
				maxExtentB := 0.0
				if bodyB.bodyType != StaticBody {
					maxExtentB = bodySimB.maxExtent
				}
				maxExtent := maxFloat(maxExtentA, maxExtentB)
				distance := Distance(xf.P, xfc.P)
				qr := InvMulRot(xf.Q, xfc.Q)

				// This metric is used for fast bodies and sleeping. It comes
				// from conservative advancement. Note that qr.s == sin(theta)
				// ~= theta for small angles. Need a tighter tolerance for
				// non-touching shapes so that contacts are not missed.
				tolerance := recycleDistanceNonTouching
				if wasTouching {
					tolerance = recycleDistance
				}
				// distance + maxExtent * abs(qr.s) < tolerance
				if distance+float64(maxExtent*absFloat(qr.S)) < tolerance {
					dqA := MulRot(transformA.Q, InvertRot(contactSim.cachedTransformA.Q))
					dqB := MulRot(transformB.Q, InvertRot(contactSim.cachedTransformB.Q))
					normal := contactSim.manifold.Normal

					// Minimize round-off
					dc := Sub(bodySimB.center, bodySimA.center)

					for i := range contactSim.manifold.PointCount {
						// Keep anchors but update separation, same as
						// sub-stepping. This eliminates jitter.
						mp := &contactSim.manifold.Points[i]
						rA := RotateVector(dqA, mp.AnchorA)
						rB := RotateVector(dqB, mp.AnchorB)
						dp := Add(dc, Sub(rB, rA))
						mp.Separation = mp.BaseSeparation + Dot(dp, normal)
						mp.Persisted = true
					}

					// Contact is recycled. This also skips updating other
					// aspects of the contact such as material parameters.
					continue
				}
			}

			contactSim.simFlags |= simRelativeTransformValid

			centerOffsetA := RotateVector(transformA.Q, bodySimA.localCenter)
			centerOffsetB := RotateVector(transformB.Q, bodySimB.localCenter)

			// This updates solid contacts
			touching := w.updateContact(contactSim, shapeA, transformA, centerOffsetA, shapeB, transformB, centerOffsetB)

			// State changes that affect island connectivity. Also affects
			// contact events.
			if touching && !wasTouching {
				contactSim.simFlags |= simStartedTouching
				setBit(&tc.contactStateBitSet, uint32(contactID))
			} else if !touching && wasTouching {
				contactSim.simFlags |= simStoppedTouching
				setBit(&tc.contactStateBitSet, uint32(contactID))
			}

			// Caching for contact recycling.
			contactSim.cachedTransformA = transformA
			contactSim.cachedTransformB = transformB
			for i := range contactSim.manifold.PointCount {
				mp := &contactSim.manifold.Points[i]
				mp.BaseSeparation = mp.Separation
			}
		}
	}
}

// addNonTouchingContact moves a contact sim into the awake set's non-touching
// array (upstream b2AddNonTouchingContact).
func (w *World) addNonTouchingContact(c *contact, contactSim *contactSim) {
	assert(c.setIndex == awakeSet)
	set := &w.solverSets[awakeSet]
	c.colorIndex = NullIndex
	c.localIndex = len(set.contactSims)

	set.contactSims = append(set.contactSims, *contactSim)
}

// removeNonTouchingContact removes a contact sim from a solver set's
// non-touching array (upstream b2RemoveNonTouchingContact).
func (w *World) removeNonTouchingContact(setIndex, localIndex int) {
	set := &w.solverSets[setIndex]
	movedIndex := removeSwap(&set.contactSims, localIndex)
	if movedIndex != NullIndex {
		movedContactSim := &set.contactSims[localIndex]
		movedContact := &w.contacts[movedContactSim.contactID]
		assert(movedContact.setIndex == setIndex)
		assert(movedContact.localIndex == movedIndex)
		assert(movedContact.colorIndex == NullIndex)
		movedContact.localIndex = localIndex
	}
}

// collide runs narrow-phase collision over all awake contacts and processes
// the touching state transitions (upstream b2Collide).
func (w *World) collide(ctx *stepContext) {
	// gather contacts into a single array for easier parallel-for
	contactCount := 0
	graphColors := &w.constraintGraph.colors
	for i := range GraphColorCount {
		contactCount += len(graphColors[i].contactSims)
	}

	nonTouchingCount := len(w.solverSets[awakeSet].contactSims)
	contactCount += nonTouchingCount

	if contactCount == 0 {
		return
	}

	contactSims := w.arena.allocContactPtrs(contactCount)

	contactIndex := 0
	for i := range GraphColorCount {
		color := &graphColors[i]
		count := len(color.contactSims)
		for j := range count {
			contactSims[contactIndex] = &color.contactSims[j]
			contactIndex++
		}
	}

	{
		awake := &w.solverSets[awakeSet]
		for i := range nonTouchingCount {
			contactSims[contactIndex] = &awake.contactSims[i]
			contactIndex++
		}
	}

	assert(contactIndex == contactCount)

	ctx.contacts = contactSims

	// Contact bit set on ids because contact pointers are unstable as they
	// move between touching and not touching.
	contactIDCapacity := getIDCapacity(&w.contactIDPool)
	setBitCountAndClear(&w.taskContext.contactStateBitSet, uint32(contactIDCapacity))

	w.collideTask(0, contactCount, ctx)
	w.taskCount++

	w.arena.freeContactPtrs()
	ctx.contacts = nil

	// Serially update contact state.
	bitSet := &w.taskContext.contactStateBitSet

	endEventArrayIndex := w.endEventArrayIndex

	worldID := w.worldID

	// Process contact state changes. Iterate over set bits.
	//
	// ORDERING CONTRACT (E6): the begin-touch transition must follow this
	// exact sequence: set contactTouchingFlag → linkContact (wakes colliding
	// bodies) → refresh the contact sim reference (the awake set may have
	// grown) → addContactToGraph → removeNonTouchingContact at the saved
	// localIndex.
	for k := range bitSet.blockCount {
		bits := bitSet.bits[k]
		for bits != 0 {
			ctz := ctz64(bits)
			contactID := int(64*k + ctz)

			c := &w.contacts[contactID]
			assert(c.setIndex == awakeSet)

			colorIndex := c.colorIndex
			localIndex := c.localIndex

			var contactSim *contactSim
			if colorIndex != NullIndex {
				// contact lives in constraint graph
				assert(0 <= colorIndex && colorIndex < GraphColorCount)
				color := &graphColors[colorIndex]
				contactSim = &color.contactSims[localIndex]
			} else {
				awake := &w.solverSets[awakeSet]
				contactSim = &awake.contactSims[localIndex]
			}

			shapeA := &w.shapes[c.shapeIDA]
			shapeB := &w.shapes[c.shapeIDB]
			shapeIDA := ShapeID{index1: int32(shapeA.id + 1), world0: worldID, generation: shapeA.generation}
			shapeIDB := ShapeID{index1: int32(shapeB.id + 1), world0: worldID, generation: shapeB.generation}
			contactFullID := ContactID{
				index1:     int32(contactID + 1),
				world0:     worldID,
				padding:    0,
				generation: c.generation,
			}
			flags := c.flags
			simFlags := contactSim.simFlags

			switch {
			case simFlags&simDisjoint != 0:
				// Bounding boxes no longer overlap
				w.destroyContact(c, false)

			case simFlags&simStartedTouching != 0:
				assert(c.islandID == NullIndex)

				if flags&contactEnableContactEvents != 0 {
					event := ContactBeginTouchEvent{ShapeIDA: shapeIDA, ShapeIDB: shapeIDB, ContactID: contactFullID}
					w.contactBeginEvents = append(w.contactBeginEvents, event)
				}

				assert(contactSim.manifold.PointCount > 0)
				assert(c.setIndex == awakeSet)

				// Link first because this wakes colliding bodies and ensures
				// the body sims are in the correct place.
				c.flags |= contactTouchingFlag
				w.linkContact(c)

				// Make sure these didn't change
				assert(c.colorIndex == NullIndex)
				assert(c.localIndex == localIndex)

				// Contact sim pointer may have become orphaned due to awake
				// set growth, so I just need to refresh it.
				awake := &w.solverSets[awakeSet]
				contactSim = &awake.contactSims[localIndex]

				contactSim.simFlags &^= simStartedTouching

				// Add first for the sim copy
				w.addContactToGraph(contactSim, c)

				// This destroys the contact sim
				w.removeNonTouchingContact(awakeSet, localIndex)

			case simFlags&simStoppedTouching != 0:
				contactSim.simFlags &^= simStoppedTouching
				c.flags &^= contactTouchingFlag

				if c.flags&contactEnableContactEvents != 0 {
					event := ContactEndTouchEvent{ShapeIDA: shapeIDA, ShapeIDB: shapeIDB, ContactID: contactFullID}
					w.contactEndEvents[endEventArrayIndex] = append(w.contactEndEvents[endEventArrayIndex], event)
				}

				assert(contactSim.manifold.PointCount == 0)

				w.unlinkContact(c)
				bodyIDA := c.edges[0].bodyID
				bodyIDB := c.edges[1].bodyID

				// Add first for the sim copy
				w.addNonTouchingContact(c, contactSim)
				w.removeContactFromGraph(bodyIDA, bodyIDB, colorIndex, localIndex)

			default:
			}

			// Clear the smallest set bit
			bits &= bits - 1
		}
	}

	w.validateSolverSets()
	w.validateContacts()
}

// Step simulates a world for one time step. This performs collision
// detection, integration, and constraint solution (upstream b2World_Step).
// timeStep is the amount of time to simulate, this should be a fixed number
// (usually 1/60). subStepCount is the number of sub-steps, increasing the
// sub-step count can increase accuracy (usually 4).
func (w *World) Step(timeStep float64, subStepCount int) {
	assert(IsValidFloat(timeStep))
	assert(0 < subStepCount)

	assert(!w.locked)
	if w.locked {
		return
	}

	// Prepare to capture events.
	// Ensure user does not access stale data if there is an early return.
	w.bodyMoveEvents = w.bodyMoveEvents[:0]
	w.sensorBeginEvents = w.sensorBeginEvents[:0]
	w.contactBeginEvents = w.contactBeginEvents[:0]
	w.contactHitEvents = w.contactHitEvents[:0]
	w.jointEvents = w.jointEvents[:0]

	// DETERMINISM: upstream fills b2Profile with wall-clock timings. The
	// timing fields stay zero in this port; opt-in timing arrives with E13.
	w.profile = Profile{}

	if timeStep == 0.0 {
		// Swap end event array buffers
		w.endEventArrayIndex = 1 - w.endEventArrayIndex
		w.sensorEndEvents[w.endEventArrayIndex] = w.sensorEndEvents[w.endEventArrayIndex][:0]
		w.contactEndEvents[w.endEventArrayIndex] = w.contactEndEvents[w.endEventArrayIndex][:0]
		return
	}

	w.locked = true
	w.taskCount = 0

	// Update collision pairs and create contacts
	w.updateBroadPhasePairs()

	ctx := stepContext{}
	ctx.world = w
	ctx.dt = timeStep
	ctx.subStepCount = maxInt(1, subStepCount)

	if timeStep > 0.0 {
		ctx.invDT = 1.0 / timeStep
		ctx.h = timeStep / float64(ctx.subStepCount)
		ctx.invH = float64(ctx.subStepCount) * ctx.invDT
	} else {
		ctx.invDT = 0.0
		ctx.h = 0.0
		ctx.invH = 0.0
	}

	w.invH = ctx.invH
	w.invDt = ctx.invDT

	// Hertz values get reduced for large time steps
	contactHertz := minFloat(w.contactHertz, 0.125*ctx.invH)
	ctx.contactSoftness = makeSoft(contactHertz, w.contactDampingRatio, ctx.h)
	ctx.staticSoftness = makeSoft(2.0*contactHertz, w.contactDampingRatio, ctx.h)

	ctx.restitutionThreshold = w.restitutionThreshold
	ctx.maxLinearVelocity = w.maxLinearSpeed
	ctx.enableWarmStarting = w.enableWarmStarting

	// Update contacts
	w.collide(&ctx)

	// Integrate velocities, solve velocity constraints, and integrate
	// positions.
	if timeStep > 0.0 {
		w.solve(&ctx)
	}

	// Update sensors.
	w.overlapSensors()

	assert(getArenaAllocation(&w.arena) == 0)

	// Swap end event array buffers
	w.endEventArrayIndex = 1 - w.endEventArrayIndex
	w.sensorEndEvents[w.endEventArrayIndex] = w.sensorEndEvents[w.endEventArrayIndex][:0]
	w.contactEndEvents[w.endEventArrayIndex] = w.contactEndEvents[w.endEventArrayIndex][:0]
	w.locked = false
}

// BodyEvents returns the body events for the current time step. The event
// data is transient — do not store references (upstream
// b2World_GetBodyEvents).
func (w *World) BodyEvents() BodyEvents {
	assert(!w.locked)
	if w.locked {
		return BodyEvents{}
	}

	return BodyEvents{MoveEvents: w.bodyMoveEvents}
}

// SensorEvents returns the sensor events for the current time step. The
// event data is transient — do not store references (upstream
// b2World_GetSensorEvents). The returned slices are valid until the next
// Step; end events come from the previous end-event buffer, which Step swaps
// after publishing.
func (w *World) SensorEvents() SensorEvents {
	assert(!w.locked)
	if w.locked {
		return SensorEvents{}
	}

	// Careful to use previous buffer
	endEventArrayIndex := 1 - w.endEventArrayIndex

	return SensorEvents{
		BeginEvents: w.sensorBeginEvents,
		EndEvents:   w.sensorEndEvents[endEventArrayIndex],
	}
}

// ContactEvents returns the contact events for the current time step. The
// event data is transient — do not store references (upstream
// b2World_GetContactEvents).
func (w *World) ContactEvents() ContactEvents {
	assert(!w.locked)
	if w.locked {
		return ContactEvents{}
	}

	// Careful to use previous buffer
	endEventArrayIndex := 1 - w.endEventArrayIndex

	return ContactEvents{
		BeginEvents: w.contactBeginEvents,
		EndEvents:   w.contactEndEvents[endEventArrayIndex],
		HitEvents:   w.contactHitEvents,
	}
}

// JointEvents returns the joint events for the current time step. The event
// data is transient — do not store references (upstream
// b2World_GetJointEvents).
func (w *World) JointEvents() JointEvents {
	assert(!w.locked)
	if w.locked {
		return JointEvents{}
	}

	return JointEvents{JointEvents: w.jointEvents}
}
