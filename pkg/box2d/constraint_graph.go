// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/constraint_graph.h, src/constraint_graph.c.
//
// Contact add/remove is ported (E6); joint add/remove is ported (E8).
//
// Deviations from upstream:
//   - The transient per-color constraint pointers (upstream a union of
//     wideConstraints/overflowConstraints) collapse to the single scalar
//     constraints slice below: this port solves every color through the
//     scalar contact solver (see contact_solver.go).
//   - b2_graphColors (debug draw palette) arrives with the debug draw stage.

package box2d

// overflowIndex holds constraints that cannot fit the graph color limit
// (upstream B2_OVERFLOW_INDEX). This happens when a single dynamic body is
// touching many other bodies.
const overflowIndex = GraphColorCount - 1

// dynamicColorCount keeps constraints involving two dynamic bodies at a lower
// solver priority than constraints involving a dynamic and static bodies
// (upstream B2_DYNAMIC_COLOR_COUNT). This reduces tunneling due to push
// through.
const dynamicColorCount = GraphColorCount - 4

// graphColor is a single constraint graph color (upstream b2GraphColor).
type graphColor struct {
	// This bitset is indexed by bodyId so this is over-sized to encompass
	// static bodies, however it is never traversed and the bit count is not
	// used for anything. This bitset is unused on the overflow color.
	bodySet bitSet

	// cache friendly arrays
	contactSims []contactSim
	jointSims   []jointSim

	// constraints is the transient scalar constraint sub-slice for this
	// color, valid only during World.solve (upstream wideConstraints /
	// overflowConstraints; this port is all-scalar, see contact_solver.go).
	// It aliases the arena contactConstraints scratch.
	constraints []contactConstraint
}

// constraintGraph is the graph coloring used to solve contacts and joints
// without synchronization (upstream b2ConstraintGraph).
type constraintGraph struct {
	// including overflow at the end
	colors [GraphColorCount]graphColor
}

// createGraph initializes a constraint graph (upstream b2CreateGraph).
func createGraph(graph *constraintGraph, bodyCapacity int) {
	*graph = constraintGraph{}

	bodyCapacity = maxInt(bodyCapacity, 8)

	// Initialize graph color bit set.
	// No bitset for overflow color.
	for i := range overflowIndex {
		color := &graph.colors[i]
		color.bodySet = createBitSet(uint32(bodyCapacity))
		setBitCountAndClear(&color.bodySet, uint32(bodyCapacity))
	}
}

// destroyGraph releases a constraint graph (upstream b2DestroyGraph).
func destroyGraph(graph *constraintGraph) {
	for i := range GraphColorCount {
		color := &graph.colors[i]
		destroyBitSet(&color.bodySet)
		color.contactSims = nil
		color.jointSims = nil
		color.constraints = nil
	}
}

// addContactToGraph assigns a touching contact to a graph color and copies
// its sim into that color (upstream b2AddContactToGraph).
//
// Contacts are always created as non-touching. They get moved into the
// constraint graph once they are found to be touching.
func (w *World) addContactToGraph(contactSim *contactSim, c *contact) {
	assert(contactSim.manifold.PointCount > 0)
	assert(contactSim.simFlags&simTouchingFlag != 0)
	assert(c.flags&contactTouchingFlag != 0)

	graph := &w.constraintGraph
	colorIndex := overflowIndex

	bodyIDA := c.edges[0].bodyID
	bodyIDB := c.edges[1].bodyID
	bodyA := &w.bodies[bodyIDA]
	bodyB := &w.bodies[bodyIDB]

	typeA := bodyA.bodyType
	typeB := bodyB.bodyType
	assert(typeA == DynamicBody || typeB == DynamicBody)

	// Upstream B2_FORCE_OVERFLOW is 0, so the color search is always active.
	switch {
	case typeA == DynamicBody && typeB == DynamicBody:
		// Dynamic constraint colors cannot encroach on colors reserved for
		// static constraints
		for i := range dynamicColorCount {
			color := &graph.colors[i]
			if getBit(&color.bodySet, uint32(bodyIDA)) || getBit(&color.bodySet, uint32(bodyIDB)) {
				continue
			}

			setBitGrow(&color.bodySet, uint32(bodyIDA))
			setBitGrow(&color.bodySet, uint32(bodyIDB))
			colorIndex = i
			break
		}
	case typeA == DynamicBody:
		// Static constraint colors build from the end to get higher priority
		// than dyn-dyn constraints
		for i := overflowIndex - 1; i >= 1; i-- {
			color := &graph.colors[i]
			if getBit(&color.bodySet, uint32(bodyIDA)) {
				continue
			}

			setBitGrow(&color.bodySet, uint32(bodyIDA))
			colorIndex = i
			break
		}
	case typeB == DynamicBody:
		// Static constraint colors build from the end to get higher priority
		// than dyn-dyn constraints
		for i := overflowIndex - 1; i >= 1; i-- {
			color := &graph.colors[i]
			if getBit(&color.bodySet, uint32(bodyIDB)) {
				continue
			}

			setBitGrow(&color.bodySet, uint32(bodyIDB))
			colorIndex = i
			break
		}
	default:
	}

	color := &graph.colors[colorIndex]
	c.colorIndex = colorIndex
	c.localIndex = len(color.contactSims)

	color.contactSims = append(color.contactSims, *contactSim)
	newContact := &color.contactSims[len(color.contactSims)-1]

	// todo perhaps skip this if the contact is already awake

	if typeA == StaticBody {
		newContact.bodySimIndexA = NullIndex
		newContact.invMassA = 0.0
		newContact.invIA = 0.0
	} else {
		assert(bodyA.setIndex == awakeSet)
		awake := &w.solverSets[awakeSet]

		localIndex := bodyA.localIndex
		newContact.bodySimIndexA = localIndex

		bodySimA := &awake.bodySims[localIndex]
		newContact.invMassA = bodySimA.invMass
		newContact.invIA = bodySimA.invInertia
	}

	if typeB == StaticBody {
		newContact.bodySimIndexB = NullIndex
		newContact.invMassB = 0.0
		newContact.invIB = 0.0
	} else {
		assert(bodyB.setIndex == awakeSet)
		awake := &w.solverSets[awakeSet]

		localIndex := bodyB.localIndex
		newContact.bodySimIndexB = localIndex

		bodySimB := &awake.bodySims[localIndex]
		newContact.invMassB = bodySimB.invMass
		newContact.invIB = bodySimB.invInertia
	}
}

// removeContactFromGraph removes a contact from its graph color
// (upstream b2RemoveContactFromGraph).
func (w *World) removeContactFromGraph(bodyIDA, bodyIDB, colorIndex, localIndex int) {
	graph := &w.constraintGraph

	assert(0 <= colorIndex && colorIndex < GraphColorCount)
	color := &graph.colors[colorIndex]

	if colorIndex != overflowIndex {
		// This might clear a bit for a kinematic or static body, but this has
		// no effect
		clearBit(&color.bodySet, uint32(bodyIDA))
		clearBit(&color.bodySet, uint32(bodyIDB))
	}

	movedIndex := removeSwap(&color.contactSims, localIndex)
	if movedIndex != NullIndex {
		// Fix index on swapped contact
		movedContactSim := &color.contactSims[localIndex]

		// Fix moved contact
		movedID := movedContactSim.contactID
		movedContact := &w.contacts[movedID]
		assert(movedContact.setIndex == awakeSet)
		assert(movedContact.colorIndex == colorIndex)
		assert(movedContact.localIndex == movedIndex)
		movedContact.localIndex = localIndex
	}
}

// assignJointColor finds a graph color for a joint and claims the body bits
// (upstream static b2AssignJointColor).
//
// Notice that a joint cannot share the same color as a contact between the
// same two bodies. This means upstream can solve contacts and joints in
// parallel with each other within each color.
func assignJointColor(graph *constraintGraph, bodyIDA, bodyIDB int, typeA, typeB BodyType) int {
	assert(typeA == DynamicBody || typeB == DynamicBody)

	// Upstream B2_FORCE_OVERFLOW is 0, so the color search is always active.
	switch {
	case typeA == DynamicBody && typeB == DynamicBody:
		// Dynamic constraint colors cannot encroach on colors reserved for
		// static constraints
		for i := range dynamicColorCount {
			color := &graph.colors[i]
			if getBit(&color.bodySet, uint32(bodyIDA)) || getBit(&color.bodySet, uint32(bodyIDB)) {
				continue
			}

			setBitGrow(&color.bodySet, uint32(bodyIDA))
			setBitGrow(&color.bodySet, uint32(bodyIDB))
			return i
		}
	case typeA == DynamicBody:
		// Static constraint colors build from the end to get higher priority
		// than dyn-dyn constraints
		for i := overflowIndex - 1; i >= 1; i-- {
			color := &graph.colors[i]
			if getBit(&color.bodySet, uint32(bodyIDA)) {
				continue
			}

			setBitGrow(&color.bodySet, uint32(bodyIDA))
			return i
		}
	case typeB == DynamicBody:
		// Static constraint colors build from the end to get higher priority
		// than dyn-dyn constraints
		for i := overflowIndex - 1; i >= 1; i-- {
			color := &graph.colors[i]
			if getBit(&color.bodySet, uint32(bodyIDB)) {
				continue
			}

			setBitGrow(&color.bodySet, uint32(bodyIDB))
			return i
		}
	default:
	}

	return overflowIndex
}

// createJointInGraph assigns a joint to a graph color and returns the new
// zero-valued joint sim in that color (upstream b2CreateJointInGraph). The
// returned pointer is valid until the color's jointSims array grows.
func (w *World) createJointInGraph(j *joint) *jointSim {
	graph := &w.constraintGraph

	bodyIDA := j.edges[0].bodyID
	bodyIDB := j.edges[1].bodyID
	bodyA := &w.bodies[bodyIDA]
	bodyB := &w.bodies[bodyIDB]

	colorIndex := assignJointColor(graph, bodyIDA, bodyIDB, bodyA.bodyType, bodyB.bodyType)

	graph.colors[colorIndex].jointSims = append(graph.colors[colorIndex].jointSims, jointSim{})

	j.colorIndex = colorIndex
	j.localIndex = len(graph.colors[colorIndex].jointSims) - 1
	return &graph.colors[colorIndex].jointSims[j.localIndex]
}

// addJointToGraph copies an existing joint sim into a graph color
// (upstream b2AddJointToGraph).
func (w *World) addJointToGraph(jointSim *jointSim, j *joint) {
	jointDst := w.createJointInGraph(j)
	*jointDst = *jointSim
}

// removeJointFromGraph removes a joint from its graph color
// (upstream b2RemoveJointFromGraph).
func (w *World) removeJointFromGraph(bodyIDA, bodyIDB, colorIndex, localIndex int) {
	graph := &w.constraintGraph

	assert(0 <= colorIndex && colorIndex < GraphColorCount)
	color := &graph.colors[colorIndex]

	if colorIndex != overflowIndex {
		// May clear static bodies, no effect
		clearBit(&color.bodySet, uint32(bodyIDA))
		clearBit(&color.bodySet, uint32(bodyIDB))
	}

	movedIndex := removeSwap(&color.jointSims, localIndex)
	if movedIndex != NullIndex {
		// Fix moved joint
		movedJointSim := &color.jointSims[localIndex]
		movedID := movedJointSim.jointID
		movedJoint := &w.joints[movedID]
		assert(movedJoint.setIndex == awakeSet)
		assert(movedJoint.colorIndex == colorIndex)
		assert(movedJoint.localIndex == movedIndex)
		movedJoint.localIndex = localIndex
	}
}
