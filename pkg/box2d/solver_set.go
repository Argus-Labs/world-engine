// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/solver_set.c, src/solver_set.h.
//
// Solver sets store sims in contiguous arrays for memory locality. The
// array iteration and swap-removal order in this file is part of the
// deterministic simulation contract — do not reorder.

package box2d

// The solver set type by index (upstream enum b2SolverSetType).
const (
	// staticSet is the set for static bodies and joints between static bodies.
	staticSet = 0

	// disabledSet is the set for disabled bodies and their joints.
	disabledSet = 1

	// awakeSet is the set for awake bodies and awake non-touching contacts.
	// Awake touching contacts and awake joints live in the constraint graph.
	awakeSet = 2

	// firstSleepingSet is the index of the first sleeping set. Each island
	// that goes to sleep is put into a sleeping set. This holds all bodies,
	// contacts, and joints from the sleeping island. A separate set for each
	// sleeping island makes it very efficient to wake a single island.
	firstSleepingSet = 3
)

// solverSet holds solver set data (upstream b2SolverSet). The following sets
// are used:
//   - static set for all static bodies and joints between static bodies
//   - active set for all active bodies with body states (no contacts or joints)
//   - disabled set for disabled bodies and their joints
//   - all further sets are sleeping island sets along with their contacts and joints
//
// The purpose of solver sets is to achieve high memory locality.
// https://www.youtube.com/watch?v=nZNd5FjSquk
type solverSet struct {
	// Body array. Empty for unused set.
	bodySims []bodySim

	// Body state only exists for active set.
	bodyStates []bodyState

	// This holds sleeping/disabled joints. Empty for static/active set.
	jointSims []jointSim

	// This holds all contacts for sleeping sets.
	// This holds non-touching contacts for the awake set.
	contactSims []contactSim

	// The awake set has an array of islands. Sleeping sets normally have a
	// single island. However, joints created between sleeping sets cause the
	// sets to merge, leaving them with multiple islands. These sleeping
	// islands will be naturally merged when the set is woken.
	// The static and disabled sets have no islands.
	// Islands live in the solver sets to limit the number of islands that
	// need to be considered for sleeping.
	islandSims []islandSim

	// Aligns with World.solverSetIDPool. Used to create a stable id for
	// body/contact/joint/islands.
	setIndex int
}

// removeSwap removes array element index by swapping in the last element and
// returns the old index of the moved element, or NullIndex if the removed
// element was last (upstream b2XxxArray_RemoveSwap from array.h).
func removeSwap[T any](array *[]T, index int) int {
	a := *array
	movedIndex := NullIndex
	if index != len(a)-1 {
		movedIndex = len(a) - 1
		a[index] = a[movedIndex]
	}

	*array = a[:len(a)-1]
	return movedIndex
}

// destroySolverSet frees a solver set and recycles its id
// (upstream b2DestroySolverSet).
func (w *World) destroySolverSet(setIndex int) {
	set := &w.solverSets[setIndex]
	set.bodySims = nil
	set.bodyStates = nil
	set.contactSims = nil
	set.jointSims = nil
	set.islandSims = nil
	freeID(&w.solverSetIDPool, setIndex)
	*set = solverSet{}
	set.setIndex = NullIndex
}

// wakeSolverSet wakes a solver set. Does not merge islands
// (upstream b2WakeSolverSet).
// Contacts can be in several places:
//  1. non-touching contacts in the disabled set
//  2. non-touching contacts already in the awake set
//  3. touching contacts in the sleeping set
//
// This handles contact types 1 and 3. Type 2 doesn't need any action.
func (w *World) wakeSolverSet(setIndex int) {
	assert(setIndex >= firstSleepingSet)
	set := &w.solverSets[setIndex]
	awake := &w.solverSets[awakeSet]
	disabled := &w.solverSets[disabledSet]

	bodies := w.bodies

	bodyCount := len(set.bodySims)
	for i := range bodyCount {
		simSrc := &set.bodySims[i]

		b := &bodies[simSrc.bodyID]
		assert(b.setIndex == setIndex)
		b.setIndex = awakeSet
		b.localIndex = len(awake.bodySims)

		// Reset sleep timer
		b.sleepTime = 0.0

		awake.bodySims = append(awake.bodySims, *simSrc)

		state := identityBodyState
		state.flags = b.flags
		awake.bodyStates = append(awake.bodyStates, state)

		// move non-touching contacts from disabled set to awake set
		contactKey := b.headContactKey
		for contactKey != NullIndex {
			edgeIndex := contactKey & 1
			contactID := contactKey >> 1

			c := &w.contacts[contactID]

			contactKey = c.edges[edgeIndex].nextKey

			if c.setIndex != disabledSet {
				assert(c.setIndex == awakeSet || c.setIndex == setIndex)
				continue
			}

			localIndex := c.localIndex
			contactSim := &disabled.contactSims[localIndex]

			assert(c.flags&contactTouchingFlag == 0 && contactSim.manifold.PointCount == 0)

			c.setIndex = awakeSet
			c.localIndex = len(awake.contactSims)
			awake.contactSims = append(awake.contactSims, *contactSim)

			movedLocalIndex := removeSwap(&disabled.contactSims, localIndex)
			if movedLocalIndex != NullIndex {
				// fix moved element
				movedContactSim := &disabled.contactSims[localIndex]
				movedContact := &w.contacts[movedContactSim.contactID]
				assert(movedContact.localIndex == movedLocalIndex)
				movedContact.localIndex = localIndex
			}
		}
	}

	// transfer touching contacts from sleeping set to contact graph
	{
		contactCount := len(set.contactSims)
		for i := range contactCount {
			contactSim := &set.contactSims[i]
			c := &w.contacts[contactSim.contactID]
			assert(c.flags&contactTouchingFlag != 0)
			assert(contactSim.simFlags&simTouchingFlag != 0)
			assert(contactSim.manifold.PointCount > 0)
			assert(c.setIndex == setIndex)
			w.addContactToGraph(contactSim, c)
			c.setIndex = awakeSet
		}
	}

	// transfer joints from sleeping set to awake set
	{
		jointCount := len(set.jointSims)
		for i := range jointCount {
			jointSim := &set.jointSims[i]
			j := &w.joints[jointSim.jointID]
			assert(j.setIndex == setIndex)
			w.addJointToGraph(jointSim, j)
			j.setIndex = awakeSet
		}
	}

	// transfer island from sleeping set to awake set
	// Usually a sleeping set has only one island, but it is possible
	// that joints are created between sleeping islands and they
	// are moved to the same sleeping set.
	{
		islandCount := len(set.islandSims)
		for i := range islandCount {
			islandSrc := set.islandSims[i]
			isl := &w.islands[islandSrc.islandID]
			isl.setIndex = awakeSet
			isl.localIndex = len(awake.islandSims)
			awake.islandSims = append(awake.islandSims, islandSrc)
		}
	}

	// destroy the sleeping set
	w.destroySolverSet(setIndex)

	w.validateSolverSets()
}

// trySleepIsland moves an island to a new sleeping solver set if it can sleep
// (upstream b2TrySleepIsland).
// Islands need to have a deterministic order because data is moved to a
// sleeping set according to island order.
func (w *World) trySleepIsland(islandID int) {
	isl := &w.islands[islandID]
	assert(isl.setIndex == awakeSet)

	// Cannot put an island to sleep while it has a pending split and more than one body.
	if isl.constraintRemoveCount > 0 && len(isl.bodies) > 1 {
		return
	}

	// island is sleeping
	// - create new sleeping solver set
	// - move island to sleeping solver set
	// - identify non-touching contacts that should move to sleeping solver set or disabled set
	// - remove old island
	// - fix island
	sleepSetID := allocID(&w.solverSetIDPool)
	if sleepSetID == len(w.solverSets) {
		set := solverSet{}
		set.setIndex = NullIndex
		w.solverSets = append(w.solverSets, set)
	}

	sleepSet := &w.solverSets[sleepSetID]
	*sleepSet = solverSet{}

	// grab awake set after creating the sleep set because the solver set
	// array may have been resized
	awake := &w.solverSets[awakeSet]
	assert(0 <= isl.localIndex && isl.localIndex < len(awake.islandSims))

	sleepSet.setIndex = sleepSetID
	sleepSet.bodySims = make([]bodySim, 0, len(isl.bodies))
	sleepSet.contactSims = make([]contactSim, 0, len(isl.contacts))
	sleepSet.jointSims = make([]jointSim, 0, len(isl.joints))

	// move awake bodies to sleeping set
	// this shuffles around bodies in the awake set
	{
		disabled := &w.solverSets[disabledSet]
		for i := range isl.bodies {
			bodyID := isl.bodies[i]
			b := &w.bodies[bodyID]
			assert(b.setIndex == awakeSet)
			assert(b.islandID == islandID)
			assert(b.islandIndex == i)

			// Update the body move event to indicate this body fell asleep.
			// It could happen the body is forced asleep before it ever moves.
			if b.bodyMoveIndex != NullIndex {
				// A move index names a slot in the buffer the finalize stage
				// filled during the last step. It stays valid for the whole
				// post-step window, which is exactly when upstream reports
				// fellAsleep — so it must NOT be blanket-invalidated on every
				// awake-set exit. It CAN dangle, though: a SetBodyType /
				// DisableBody / joint-driven transfer out of the awake set
				// leaves an index into a buffer that a later step reslices
				// shorter or refills for a different body. Upstream indexes it
				// unchecked and reads out of bounds (found by the E14
				// op-sequence fuzzer), so validate bounds and event identity
				// here rather than dropping the notification at the source.
				if b.bodyMoveIndex < len(w.bodyMoveEvents) {
					moveEvent := &w.bodyMoveEvents[b.bodyMoveIndex]
					if int(moveEvent.BodyID.index1)-1 == bodyID && moveEvent.BodyID.generation == b.generation {
						moveEvent.FellAsleep = true
					}
				}
				b.bodyMoveIndex = NullIndex
			}

			awakeBodyIndex := b.localIndex
			awakeSim := &awake.bodySims[awakeBodyIndex]

			// move body sim to sleep set
			sleepBodyIndex := len(sleepSet.bodySims)
			sleepSet.bodySims = append(sleepSet.bodySims, *awakeSim)

			removeBodySim(&awake.bodySims, w.bodies, awakeBodyIndex)

			// destroy state, no need to clone
			_ = removeSwap(&awake.bodyStates, awakeBodyIndex)

			b.setIndex = sleepSetID
			b.localIndex = sleepBodyIndex

			// Move non-touching contacts to the disabled set.
			// Non-touching contacts may exist between sleeping islands and
			// there is no clear ownership.
			contactKey := b.headContactKey
			for contactKey != NullIndex {
				contactID := contactKey >> 1
				edgeIndex := contactKey & 1

				c := &w.contacts[contactID]

				assert(c.setIndex == awakeSet || c.setIndex == disabledSet)
				contactKey = c.edges[edgeIndex].nextKey

				if c.setIndex == disabledSet {
					// already moved to disabled set by another body in the island
					continue
				}

				if c.colorIndex != NullIndex {
					// contact is touching and will be moved separately
					assert(c.flags&contactTouchingFlag != 0)
					continue
				}

				// the other body may still be awake, it still may go to sleep
				// and then it will be responsible for moving this contact to
				// the disabled set.
				otherEdgeIndex := edgeIndex ^ 1
				otherBodyID := c.edges[otherEdgeIndex].bodyID
				otherBody := &w.bodies[otherBodyID]
				if otherBody.setIndex == awakeSet {
					continue
				}

				localIndex := c.localIndex
				contactSim := &awake.contactSims[localIndex]

				assert(contactSim.manifold.PointCount == 0)
				assert(c.flags&contactTouchingFlag == 0)

				// move the non-touching contact to the disabled set
				c.setIndex = disabledSet
				c.localIndex = len(disabled.contactSims)
				disabled.contactSims = append(disabled.contactSims, *contactSim)

				movedLocalIndex := removeSwap(&awake.contactSims, localIndex)
				if movedLocalIndex != NullIndex {
					// fix moved element
					movedContactSim := &awake.contactSims[localIndex]
					movedContact := &w.contacts[movedContactSim.contactID]
					assert(movedContact.localIndex == movedLocalIndex)
					movedContact.localIndex = localIndex
				}
			}
		}
	}

	// move touching contacts
	// this shuffles contacts in the awake set
	{
		for i := range isl.contacts {
			link := &isl.contacts[i]
			c := &w.contacts[link.contactID]
			assert(c.setIndex == awakeSet)
			assert(c.islandID == islandID)
			colorIndex := c.colorIndex
			assert(0 <= colorIndex && colorIndex < GraphColorCount)

			color := &w.constraintGraph.colors[colorIndex]

			// Remove bodies from graph coloring associated with this constraint
			if colorIndex != overflowIndex {
				// might clear a bit for a static body, but this has no effect
				clearBit(&color.bodySet, uint32(c.edges[0].bodyID))
				clearBit(&color.bodySet, uint32(c.edges[1].bodyID))
			}

			localIndex := c.localIndex
			awakeContactSim := &color.contactSims[localIndex]

			sleepContactIndex := len(sleepSet.contactSims)
			sleepSet.contactSims = append(sleepSet.contactSims, *awakeContactSim)

			movedLocalIndex := removeSwap(&color.contactSims, localIndex)
			if movedLocalIndex != NullIndex {
				// fix moved element
				movedContactSim := &color.contactSims[localIndex]
				movedContact := &w.contacts[movedContactSim.contactID]
				assert(movedContact.localIndex == movedLocalIndex)
				movedContact.localIndex = localIndex
			}

			c.setIndex = sleepSetID
			c.colorIndex = NullIndex
			c.localIndex = sleepContactIndex
		}
	}

	// move joints
	// this shuffles joints in the awake set
	{
		for i := range isl.joints {
			link := &isl.joints[i]
			j := &w.joints[link.jointID]
			assert(j.setIndex == awakeSet)
			assert(j.islandID == islandID)
			colorIndex := j.colorIndex
			localIndex := j.localIndex

			assert(0 <= colorIndex && colorIndex < GraphColorCount)

			color := &w.constraintGraph.colors[colorIndex]

			awakeJointSim := &color.jointSims[localIndex]

			if colorIndex != overflowIndex {
				// might clear a bit for a static body, but this has no effect
				clearBit(&color.bodySet, uint32(j.edges[0].bodyID))
				clearBit(&color.bodySet, uint32(j.edges[1].bodyID))
			}

			sleepJointIndex := len(sleepSet.jointSims)
			sleepSet.jointSims = append(sleepSet.jointSims, *awakeJointSim)

			movedIndex := removeSwap(&color.jointSims, localIndex)
			if movedIndex != NullIndex {
				// fix moved element
				movedJointSim := &color.jointSims[localIndex]
				movedID := movedJointSim.jointID
				movedJoint := &w.joints[movedID]
				assert(movedJoint.localIndex == movedIndex)
				movedJoint.localIndex = localIndex
			}

			j.setIndex = sleepSetID
			j.colorIndex = NullIndex
			j.localIndex = sleepJointIndex
		}
	}

	// move island struct
	{
		assert(isl.setIndex == awakeSet)

		islandIndex := isl.localIndex
		sleepSet.islandSims = append(sleepSet.islandSims, islandSim{islandID: islandID})

		movedIslandIndex := removeSwap(&awake.islandSims, islandIndex)
		if movedIslandIndex != NullIndex {
			// fix index on moved element
			movedIslandSim := &awake.islandSims[islandIndex]
			movedIslandID := movedIslandSim.islandID
			movedIsland := &w.islands[movedIslandID]
			assert(movedIsland.localIndex == movedIslandIndex)
			movedIsland.localIndex = islandIndex
		}

		isl.setIndex = sleepSetID
		isl.localIndex = 0
	}

	if w.splitIslandID == islandID {
		w.splitIslandID = NullIndex
	}

	w.validateSolverSets()
}

// mergeSolverSets merges set 2 into set 1 then destroys set 2
// (upstream b2MergeSolverSets).
// This is called when joints are created between sets. It allows the sets to
// continue sleeping if both are asleep. Otherwise one set is woken. Islands
// get merged when the set is woken.
//
// Warning: any pointers into these sets will be orphaned.
func (w *World) mergeSolverSets(setID1, setID2 int) {
	assert(setID1 >= firstSleepingSet)
	assert(setID2 >= firstSleepingSet)
	set1 := &w.solverSets[setID1]
	set2 := &w.solverSets[setID2]

	// Move the fewest number of bodies
	if len(set1.bodySims) < len(set2.bodySims) {
		set1, set2 = set2, set1
		setID1, setID2 = setID2, setID1
	}

	// transfer bodies
	{
		bodies := w.bodies
		bodyCount := len(set2.bodySims)
		for i := range bodyCount {
			simSrc := &set2.bodySims[i]

			b := &bodies[simSrc.bodyID]
			assert(b.setIndex == setID2)
			b.setIndex = setID1
			b.localIndex = len(set1.bodySims)

			set1.bodySims = append(set1.bodySims, *simSrc)
		}
	}

	// transfer contacts
	{
		contactCount := len(set2.contactSims)
		for i := range contactCount {
			contactSrc := &set2.contactSims[i]

			c := &w.contacts[contactSrc.contactID]
			assert(c.setIndex == setID2)
			c.setIndex = setID1
			c.localIndex = len(set1.contactSims)

			set1.contactSims = append(set1.contactSims, *contactSrc)
		}
	}

	// transfer joints
	{
		jointCount := len(set2.jointSims)
		for i := range jointCount {
			jointSrc := &set2.jointSims[i]

			j := &w.joints[jointSrc.jointID]
			assert(j.setIndex == setID2)
			j.setIndex = setID1
			j.localIndex = len(set1.jointSims)

			set1.jointSims = append(set1.jointSims, *jointSrc)
		}
	}

	// transfer islands
	{
		islandCount := len(set2.islandSims)
		for i := range islandCount {
			islandSrc := set2.islandSims[i]
			islandID := islandSrc.islandID

			isl := &w.islands[islandID]
			isl.setIndex = setID1
			isl.localIndex = len(set1.islandSims)

			set1.islandSims = append(set1.islandSims, islandSrc)
		}
	}

	// destroy the merged set
	w.destroySolverSet(setID2)

	w.validateSolverSets()
}

// transferBody moves a body sim between solver sets (upstream b2TransferBody).
func (w *World) transferBody(targetSet, sourceSet *solverSet, b *body) {
	if targetSet == sourceSet {
		return
	}

	sourceIndex := b.localIndex
	sourceSim := &sourceSet.bodySims[sourceIndex]

	targetIndex := len(targetSet.bodySims)
	targetSet.bodySims = append(targetSet.bodySims, *sourceSim)
	targetSim := &targetSet.bodySims[targetIndex]

	// Clear transient body flags
	targetSim.flags &^= isFast | isSpeedCapped | hadTimeOfImpact

	// Remove body sim from solver set that owns it
	removeBodySim(&sourceSet.bodySims, w.bodies, sourceIndex)

	if sourceSet.setIndex == awakeSet {
		_ = removeSwap(&sourceSet.bodyStates, sourceIndex)
	} else if targetSet.setIndex == awakeSet {
		state := identityBodyState
		state.flags = b.flags
		targetSet.bodyStates = append(targetSet.bodyStates, state)
	}

	b.setIndex = targetSet.setIndex
	b.localIndex = targetIndex
}

// transferJoint moves a joint sim between solver sets
// (upstream b2TransferJoint).
func (w *World) transferJoint(targetSet, sourceSet *solverSet, j *joint) {
	if targetSet == sourceSet {
		return
	}

	localIndex := j.localIndex
	colorIndex := j.colorIndex

	// Retrieve source.
	var sourceSim *jointSim
	if sourceSet.setIndex == awakeSet {
		assert(0 <= colorIndex && colorIndex < GraphColorCount)
		color := &w.constraintGraph.colors[colorIndex]

		sourceSim = &color.jointSims[localIndex]
	} else {
		assert(colorIndex == NullIndex)
		sourceSim = &sourceSet.jointSims[localIndex]
	}

	// Create target and copy. Fix joint.
	if targetSet.setIndex == awakeSet {
		w.addJointToGraph(sourceSim, j)
		j.setIndex = awakeSet
	} else {
		j.setIndex = targetSet.setIndex
		j.localIndex = len(targetSet.jointSims)
		j.colorIndex = NullIndex

		targetSet.jointSims = append(targetSet.jointSims, *sourceSim)
	}

	// Destroy source.
	if sourceSet.setIndex == awakeSet {
		w.removeJointFromGraph(j.edges[0].bodyID, j.edges[1].bodyID, colorIndex, localIndex)
	} else {
		movedIndex := removeSwap(&sourceSet.jointSims, localIndex)
		if movedIndex != NullIndex {
			// fix swapped element
			movedJointSim := &sourceSet.jointSims[localIndex]
			movedID := movedJointSim.jointID
			movedJoint := &w.joints[movedID]
			movedJoint.localIndex = localIndex
		}
	}
}
