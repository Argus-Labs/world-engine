// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/island.h, src/island.c.
//
// Island link/unlink for contacts, island splitting (E7) and joint
// link/unlink (E8) are ported.
//
// Deviations from upstream:
//   - b2ValidateIsland is ported behind debugAsserts (upstream compiles it
//     out unless B2_ENABLE_VALIDATION).

package box2d

// contactLink is cached contact data stored in the island for fast contiguous
// iteration (upstream b2ContactLink). Avoids touching contact during
// union-find in splitIsland.
type contactLink struct {
	contactID int
	bodyIDA   int
	bodyIDB   int
}

// jointLink is cached joint data stored in the island for fast contiguous
// iteration (upstream b2JointLink).
type jointLink struct {
	jointID int
	bodyIDA int
	bodyIDB int
}

// island is a persistent island for awake bodies, joints, and contacts
// (upstream b2Island). Contacts are touching. Contacts and joints may connect
// to static bodies, but static bodies are not in the island.
// https://en.wikipedia.org/wiki/Component_(graph_theory)
// https://en.wikipedia.org/wiki/Dynamic_connectivity
type island struct {
	// index of solver set stored in World; may be NullIndex
	setIndex int

	// island index within set; may be NullIndex
	localIndex int

	islandID int

	// Keeps track of how many contacts have been removed from this island.
	// This is used to determine if an island is a candidate for splitting.
	constraintRemoveCount int

	// Body ids in this island.
	bodies []int

	// Contacts and joints that belong to this island. May connect to static
	// bodies not in the island. Each link has the two body ids so that
	// splitIsland's union-find pass never needs to touch contact/joint.
	contacts []contactLink
	joints   []jointLink
}

// islandSim is used to move islands across solver sets (upstream b2IslandSim).
type islandSim struct {
	islandID int
}

// createIsland creates a new island in the given solver set and returns a
// pointer to it (upstream b2CreateIsland). The pointer is valid until the
// world island array grows.
func (w *World) createIsland(setIndex int) *island {
	assert(setIndex == awakeSet || setIndex >= firstSleepingSet)

	islandID := allocID(&w.islandIDPool)

	if islandID == len(w.islands) {
		w.islands = append(w.islands, island{})
	} else {
		assert(w.islands[islandID].setIndex == NullIndex)
	}

	set := &w.solverSets[setIndex]

	isl := &w.islands[islandID]
	isl.setIndex = setIndex
	isl.localIndex = len(set.islandSims)
	isl.islandID = islandID
	isl.bodies = nil
	isl.contacts = nil
	isl.joints = nil
	isl.constraintRemoveCount = 0

	set.islandSims = append(set.islandSims, islandSim{islandID: islandID})

	return isl
}

// destroyIsland frees an island that is assumed to be empty
// (upstream b2DestroyIsland).
func (w *World) destroyIsland(islandID int) {
	if w.splitIslandID == islandID {
		w.splitIslandID = NullIndex
	}

	// assume island is empty
	isl := &w.islands[islandID]
	set := &w.solverSets[isl.setIndex]
	{
		localIndex := isl.localIndex
		lastIndex := len(set.islandSims) - 1
		assert(0 <= localIndex && localIndex <= lastIndex)
		moveIslandID := set.islandSims[lastIndex].islandID
		set.islandSims[localIndex] = set.islandSims[lastIndex]
		w.islands[moveIslandID].localIndex = localIndex
		set.islandSims = set.islandSims[:lastIndex]
	}

	// Free island and id (preserve island revision)
	isl.bodies = nil
	isl.contacts = nil
	isl.joints = nil
	isl.constraintRemoveCount = 0
	isl.localIndex = NullIndex
	isl.islandID = NullIndex
	isl.setIndex = NullIndex

	freeID(&w.islandIDPool, islandID)
}

// mergeIslands merges two islands and returns the id of the island that
// survived. This destroys the smaller island (upstream static b2MergeIslands).
func (w *World) mergeIslands(islandIDA, islandIDB int) int {
	if islandIDA == islandIDB {
		return islandIDA
	}

	if islandIDA == NullIndex {
		assert(islandIDB != NullIndex)
		return islandIDB
	}

	if islandIDB == NullIndex {
		assert(islandIDA != NullIndex)
		return islandIDA
	}

	var smallIsland, bigIsland *island
	{
		islandA := &w.islands[islandIDA]
		islandB := &w.islands[islandIDB]

		// Keep the biggest island to reduce cache misses
		if len(islandA.bodies) >= len(islandB.bodies) {
			bigIsland = islandA
			smallIsland = islandB
		} else {
			bigIsland = islandB
			smallIsland = islandA
		}
	}

	bigIslandID := bigIsland.islandID

	// Move bodies from smaller island to larger island
	for i := range smallIsland.bodies {
		bodyID := smallIsland.bodies[i]
		b := &w.bodies[bodyID]
		// upstream B2_VALIDATE
		assert(b.islandID == smallIsland.islandID)
		b.islandID = bigIslandID
		b.islandIndex = len(bigIsland.bodies)
		bigIsland.bodies = append(bigIsland.bodies, bodyID)
	}

	// Migrate contacts from smaller island to larger island
	if len(smallIsland.contacts) > 0 {
		for i := range smallIsland.contacts {
			link := smallIsland.contacts[i]
			c := &w.contacts[link.contactID]
			c.islandID = bigIslandID
			c.islandIndex = len(bigIsland.contacts)
			bigIsland.contacts = append(bigIsland.contacts, link)
		}
	}

	// Migrate joints from smaller island to larger island
	if len(smallIsland.joints) > 0 {
		for i := range smallIsland.joints {
			link := smallIsland.joints[i]
			j := &w.joints[link.jointID]
			j.islandID = bigIslandID
			j.islandIndex = len(bigIsland.joints)
			bigIsland.joints = append(bigIsland.joints, link)
		}
	}

	// Track removed constraints
	bigIsland.constraintRemoveCount += smallIsland.constraintRemoveCount

	w.destroyIsland(smallIsland.islandID)

	w.validateIsland(bigIslandID)

	return bigIslandID
}

// addContactToIsland adds a contact to an island (upstream static
// b2AddContactToIsland).
func (w *World) addContactToIsland(islandID int, c *contact) {
	assert(c.islandID == NullIndex)
	assert(c.islandIndex == NullIndex)

	isl := &w.islands[islandID]

	c.islandID = islandID
	c.islandIndex = len(isl.contacts)

	link := contactLink{
		contactID: c.contactID,
		bodyIDA:   c.edges[0].bodyID,
		bodyIDB:   c.edges[1].bodyID,
	}
	isl.contacts = append(isl.contacts, link)

	w.validateIsland(islandID)
}

// linkContact links a touching contact into an island, merging islands as
// needed (upstream b2LinkContact).
func (w *World) linkContact(c *contact) {
	assert(c.flags&contactTouchingFlag != 0)

	bodyIDA := c.edges[0].bodyID
	bodyIDB := c.edges[1].bodyID

	bodyA := &w.bodies[bodyIDA]
	bodyB := &w.bodies[bodyIDB]

	assert(bodyA.setIndex != disabledSet && bodyB.setIndex != disabledSet)
	assert(bodyA.setIndex != staticSet || bodyB.setIndex != staticSet)

	// Wake bodyB if bodyA is awake and bodyB is sleeping
	if bodyA.setIndex == awakeSet && bodyB.setIndex >= firstSleepingSet {
		w.wakeSolverSet(bodyB.setIndex)
	}

	// Wake bodyA if bodyB is awake and bodyA is sleeping
	if bodyB.setIndex == awakeSet && bodyA.setIndex >= firstSleepingSet {
		w.wakeSolverSet(bodyA.setIndex)
	}

	islandIDA := bodyA.islandID
	islandIDB := bodyB.islandID

	// Static bodies have null island indices.
	assert(bodyA.setIndex != staticSet || islandIDA == NullIndex)
	assert(bodyB.setIndex != staticSet || islandIDB == NullIndex)
	assert(islandIDA != NullIndex || islandIDB != NullIndex)

	// Merge islands. This will destroy one of the islands.
	finalIslandID := w.mergeIslands(islandIDA, islandIDB)

	// Add contact to the island that survived
	w.addContactToIsland(finalIslandID, c)
}

// unlinkContact unlinks a contact from its island. This is called when a
// contact no longer has contact points or when a contact is destroyed
// (upstream b2UnlinkContact).
func (w *World) unlinkContact(c *contact) {
	assert(c.islandID != NullIndex)

	// remove from island
	islandID := c.islandID
	isl := &w.islands[islandID]

	removeIndex := c.islandIndex
	assert(0 <= removeIndex && removeIndex < len(isl.contacts))
	assert(isl.contacts[removeIndex].contactID == c.contactID)

	movedIndex := removeSwap(&isl.contacts, removeIndex)
	if movedIndex != NullIndex {
		// Fix islandIndex on the contact that was swapped into removeIndex
		movedLink := &isl.contacts[removeIndex]
		movedContact := &w.contacts[movedLink.contactID]
		assert(movedContact.islandIndex == movedIndex)
		movedContact.islandIndex = removeIndex
	}

	c.islandID = NullIndex
	c.islandIndex = NullIndex
	isl.constraintRemoveCount++

	w.validateIsland(islandID)
}

// addJointToIsland adds a joint to an island (upstream static
// b2AddJointToIsland).
func (w *World) addJointToIsland(islandID int, j *joint) {
	assert(j.islandID == NullIndex)
	assert(j.islandIndex == NullIndex)

	isl := &w.islands[islandID]

	j.islandID = islandID
	j.islandIndex = len(isl.joints)

	link := jointLink{
		jointID: j.jointID,
		bodyIDA: j.edges[0].bodyID,
		bodyIDB: j.edges[1].bodyID,
	}
	isl.joints = append(isl.joints, link)

	w.validateIsland(islandID)
}

// linkJoint links a joint into the island graph, merging islands as needed
// (upstream b2LinkJoint).
func (w *World) linkJoint(j *joint) {
	bodyA := &w.bodies[j.edges[0].bodyID]
	bodyB := &w.bodies[j.edges[1].bodyID]

	assert(bodyA.bodyType == DynamicBody || bodyB.bodyType == DynamicBody)

	if bodyA.setIndex == awakeSet && bodyB.setIndex >= firstSleepingSet {
		w.wakeSolverSet(bodyB.setIndex)
	} else if bodyB.setIndex == awakeSet && bodyA.setIndex >= firstSleepingSet {
		w.wakeSolverSet(bodyA.setIndex)
	}

	islandIDA := bodyA.islandID
	islandIDB := bodyB.islandID

	assert(islandIDA != NullIndex || islandIDB != NullIndex)

	// Merge islands. This will destroy one of the islands.
	finalIslandID := w.mergeIslands(islandIDA, islandIDB)

	// Add joint the island that survived
	w.addJointToIsland(finalIslandID, j)
}

// unlinkJoint unlinks a joint from its island
// (upstream b2UnlinkJoint).
func (w *World) unlinkJoint(j *joint) {
	if j.islandID == NullIndex {
		return
	}

	// remove from island
	islandID := j.islandID
	isl := &w.islands[islandID]

	removeIndex := j.islandIndex
	assert(0 <= removeIndex && removeIndex < len(isl.joints))
	assert(isl.joints[removeIndex].jointID == j.jointID)

	movedIndex := removeSwap(&isl.joints, removeIndex)
	if movedIndex != NullIndex {
		// Fix islandIndex on the joint that was swapped into removeIndex
		movedLink := &isl.joints[removeIndex]
		movedJoint := &w.joints[movedLink.jointID]
		assert(movedJoint.islandIndex == movedIndex)
		movedJoint.islandIndex = removeIndex
	}

	j.islandID = NullIndex
	j.islandIndex = NullIndex
	isl.constraintRemoveCount++

	w.validateIsland(islandID)
}

// islandFindParent finds the parent of a node. Uses path halving to speed up
// further queries (upstream b2IslandFindParent).
func islandFindParent(parents []int, node int) int {
	// Walk the chain of parents to find the node that is its own parent (the
	// root).
	for parents[node] != node {
		grandParent := parents[parents[node]]
		parents[node] = grandParent
		node = grandParent
	}

	return node
}

// islandUnion connects the components containing node1 and node2. Uses rank
// to keep the tree balanced. Tracks per-component contact and joint counts
// (upstream b2IslandUnion).
func islandUnion(parents, ranks []int, node1, node2 int, contactCounts, jointCounts []int) {
	root1 := islandFindParent(parents, node1)
	root2 := islandFindParent(parents, node2)
	if root1 != root2 {
		switch {
		case ranks[root1] < ranks[root2]:
			parents[root1] = root2
			contactCounts[root2] += contactCounts[root1]
			jointCounts[root2] += jointCounts[root1]
		case ranks[root1] > ranks[root2]:
			parents[root2] = root1
			contactCounts[root1] += contactCounts[root2]
			jointCounts[root1] += jointCounts[root2]
		default:
			parents[root2] = root1
			ranks[root1]++
			contactCounts[root1] += contactCounts[root2]
			jointCounts[root1] += jointCounts[root2]
		}
	}
}

// splitIsland splits an island into connected components using union-find
// over the island body/contact/joint links (upstream b2SplitIsland).
//
// This is called during the solve while islands are not being touched. It
// uses union find and touches a lot of memory, so it can be slow.
// Note: contacts/joints connected to static bodies must belong to an island
// but don't affect island connectivity.
// Note: static bodies are never in an island.
func (w *World) splitIsland(baseID int) {
	baseIsland := &w.islands[baseID]
	assert(baseIsland.constraintRemoveCount > 0)
	assert(baseIsland.setIndex == awakeSet)

	w.validateIsland(baseID)

	// Cache base island arrays before createIsland, which may reallocate
	// w.islands and invalidate the baseIsland pointer.
	baseBodyCount := len(baseIsland.bodies)
	baseBodyIDs := baseIsland.bodies

	baseContactCount := len(baseIsland.contacts)
	baseContacts := baseIsland.contacts

	baseJointCount := len(baseIsland.joints)
	baseJoints := baseIsland.joints

	alloc := &w.arena

	// Allocate contactCounts and jointCounts before ranks so ranks can be
	// freed first (LIFO arena upstream).
	parents := alloc.allocInts(&alloc.splitParents, baseBodyCount)
	contactCounts := alloc.allocInts(&alloc.splitContactCounts, baseBodyCount)
	jointCounts := alloc.allocInts(&alloc.splitJointCounts, baseBodyCount)
	ranks := alloc.allocInts(&alloc.splitRanks, baseBodyCount)
	for i := range baseBodyCount {
		parents[i] = i
		ranks[i] = 0
		contactCounts[i] = 0
		jointCounts[i] = 0
	}

	bodies := w.bodies

	// Union over contacts, tracking per-component contact counts
	for i := range baseContactCount {
		bodyIDA := baseContacts[i].bodyIDA
		bodyIDB := baseContacts[i].bodyIDB
		bodyA := &bodies[bodyIDA]
		bodyB := &bodies[bodyIDB]
		islandIndexA := bodyA.islandIndex
		islandIndexB := bodyB.islandIndex

		// Only connect non-static bodies
		if islandIndexA != NullIndex && islandIndexB != NullIndex {
			islandUnion(parents, ranks, islandIndexA, islandIndexB, contactCounts, jointCounts)
			root := islandFindParent(parents, islandIndexA)
			contactCounts[root]++
		} else {
			islandIndex := islandIndexB
			if islandIndexA != NullIndex {
				islandIndex = islandIndexA
			}
			root := islandFindParent(parents, islandIndex)
			contactCounts[root]++
		}
	}

	// Union over joints, tracking per-component joint counts
	for i := range baseJointCount {
		bodyIDA := baseJoints[i].bodyIDA
		bodyIDB := baseJoints[i].bodyIDB
		bodyA := &bodies[bodyIDA]
		bodyB := &bodies[bodyIDB]
		islandIndexA := bodyA.islandIndex
		islandIndexB := bodyB.islandIndex

		// Only connect non-static bodies
		if islandIndexA != NullIndex && islandIndexB != NullIndex {
			islandUnion(parents, ranks, islandIndexA, islandIndexB, contactCounts, jointCounts)
			root := islandFindParent(parents, islandIndexA)
			jointCounts[root]++
		} else {
			islandIndex := islandIndexB
			if islandIndexA != NullIndex {
				islandIndex = islandIndexA
			}
			root := islandFindParent(parents, islandIndex)
			jointCounts[root]++
		}
	}

	// Done with ranks
	alloc.freeInts(&alloc.splitRanks)

	// Flatten all parent indices and count connected components.
	componentCount := 0
	for i := range baseBodyCount {
		parents[i] = islandFindParent(parents, i)
		if parents[i] == i {
			componentCount++
		}
	}

	// Early return — island is still fully connected, no split needed.
	if componentCount == 1 {
		baseIsland.constraintRemoveCount = 0
		alloc.freeInts(&alloc.splitJointCounts)
		alloc.freeInts(&alloc.splitContactCounts)
		alloc.freeInts(&alloc.splitParents)
		return
	}

	// Detach body/contact/joint arrays from the base island so destroyIsland
	// won't touch them.
	baseIsland.bodies = nil
	baseIsland.contacts = nil
	baseIsland.joints = nil

	// Nil so code below doesn't accidentally use this.
	baseIsland = nil
	_ = baseIsland

	// Map from body index to new island index. Only set for root bodies.
	rootMap := alloc.allocInts(&alloc.splitRootMap, baseBodyCount)
	for i := range baseBodyCount {
		rootMap[i] = NullIndex
	}

	componentBodyCounts := alloc.allocInts(&alloc.splitComponentBodyCounts, componentCount)
	componentContactCounts := alloc.allocInts(&alloc.splitComponentContactCounts, componentCount)
	componentJointCounts := alloc.allocInts(&alloc.splitComponentJointCounts, componentCount)
	islandCount := 0

	// Find the root body for each body and create islands as needed.
	// Extract per-component counts from the root nodes' accumulated counts.
	for i := range baseBodyCount {
		rootIndex := parents[i]
		if rootMap[rootIndex] == NullIndex {
			rootMap[rootIndex] = islandCount
			componentBodyCounts[islandCount] = 0
			componentContactCounts[islandCount] = contactCounts[rootIndex]
			componentJointCounts[islandCount] = jointCounts[rootIndex]
			islandCount++
		}

		componentBodyCounts[rootMap[rootIndex]]++
	}

	assert(islandCount == componentCount)

	// Map from new island index to island id
	islandIDs := alloc.allocInts(&alloc.splitIslandIDs, islandCount)

	// Create new islands and reserve body/contact/joint arrays
	for i := range islandCount {
		// WARNING: this invalidates the baseIsland pointer
		newIsland := w.createIsland(awakeSet)
		islandIDs[i] = newIsland.islandID

		// Reserve arrays to avoid wasteful growth.
		newIsland.bodies = make([]int, 0, componentBodyCounts[i])
		newIsland.contacts = make([]contactLink, 0, componentContactCounts[i])
		newIsland.joints = make([]jointLink, 0, componentJointCounts[i])
	}

	// Assign bodies to new islands
	for i := range baseBodyCount {
		bodyID := baseBodyIDs[i]
		root := islandFindParent(parents, i)
		newIslandID := islandIDs[rootMap[root]]

		b := &w.bodies[bodyID]
		newIsland := &w.islands[newIslandID]

		b.islandID = newIslandID
		b.islandIndex = len(newIsland.bodies)

		newIsland.bodies = append(newIsland.bodies, bodyID)
	}

	// Assign contacts to the island of their bodies
	for i := range baseContactCount {
		link := baseContacts[i]
		c := &w.contacts[link.contactID]

		// Static bodies don't have an island id.
		bodyA := &w.bodies[link.bodyIDA]
		bodyB := &w.bodies[link.bodyIDB]
		targetIslandID := bodyB.islandID
		if bodyA.islandID != NullIndex {
			targetIslandID = bodyA.islandID
		}

		targetIsland := &w.islands[targetIslandID]
		c.islandID = targetIslandID
		c.islandIndex = len(targetIsland.contacts)

		targetIsland.contacts = append(targetIsland.contacts, link)
	}

	// Assign joints to the island of their bodies
	for i := range baseJointCount {
		link := baseJoints[i]
		j := &w.joints[link.jointID]

		// Static bodies don't have an island id.
		bodyA := &w.bodies[link.bodyIDA]
		bodyB := &w.bodies[link.bodyIDB]
		targetIslandID := bodyB.islandID
		if bodyA.islandID != NullIndex {
			targetIslandID = bodyA.islandID
		}

		targetIsland := &w.islands[targetIslandID]
		j.islandID = targetIslandID
		j.islandIndex = len(targetIsland.joints)

		targetIsland.joints = append(targetIsland.joints, link)
	}

	// Destroy the base island
	w.destroyIsland(baseID)

	// The detached base arrays are garbage collected (upstream frees them
	// manually here).

	// Free arena items in LIFO order
	alloc.freeInts(&alloc.splitIslandIDs)
	alloc.freeInts(&alloc.splitComponentJointCounts)
	alloc.freeInts(&alloc.splitComponentContactCounts)
	alloc.freeInts(&alloc.splitComponentBodyCounts)
	alloc.freeInts(&alloc.splitRootMap)
	alloc.freeInts(&alloc.splitJointCounts)
	alloc.freeInts(&alloc.splitContactCounts)
	alloc.freeInts(&alloc.splitParents)
}

// validateIsland mirrors b2ValidateIsland. The checks only run when
// debugAsserts is enabled, matching upstream B2_ENABLE_VALIDATION builds;
// release builds compile this out (see core.go).
func (w *World) validateIsland(islandID int) {
	if !debugAsserts {
		return
	}

	if islandID == NullIndex {
		return
	}

	isl := &w.islands[islandID]
	assert(isl.islandID == islandID)
	assert(isl.setIndex != NullIndex)

	{
		assert(len(isl.bodies) > 0)
		assert(len(isl.bodies) <= getIDCount(&w.bodyIDPool))

		for i := range isl.bodies {
			b := &w.bodies[isl.bodies[i]]
			assert(b.islandID == islandID)
			assert(b.islandIndex == i)
			assert(b.setIndex == isl.setIndex)
		}
	}

	if len(isl.contacts) > 0 {
		assert(len(isl.contacts) <= getIDCount(&w.contactIDPool))

		for i := range isl.contacts {
			link := &isl.contacts[i]
			c := &w.contacts[link.contactID]
			assert(c.setIndex == isl.setIndex)
			assert(c.islandID == islandID)
			assert(c.islandIndex == i)
		}
	}

	if len(isl.joints) > 0 {
		assert(len(isl.joints) <= getIDCount(&w.jointIDPool))

		for i := range isl.joints {
			link := &isl.joints[i]
			j := &w.joints[link.jointID]
			assert(j.setIndex == isl.setIndex)
			assert(j.islandID == islandID)
			assert(j.islandIndex == i)
		}
	}
}
