package ecs

import (
	"slices"
	"strings"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/kelindar/bitmap"
)

// archetypeID is the unique identifier for an archetype.
// It is used internally to track and manage archetypes efficiently.
type archetypeID = int

// archetype represents a collection of entities with the same component types.
// NOTE: We store the compCount instead of using Bitmap.Count() because counting bits is O(n). This
// saves us around 10ns/op, which is 10x speed up in low # of components. We store columns in a
// slice instead of a map because it's faster for small # of components.
type archetype struct {
	id         archetypeID      // Corresponds to the index in the archetypes array
	components bitmap.Bitmap    // Bitmap of components contained in this archetype
	rows       sparseSet        // Maps entity ID -> row index in entities and columns
	entities   []EntityID       // List of entities of this archetype
	columns    []abstractColumn // List of columns containing component data
	compCount  int              // Number of component types in the archetype

	// wireOrder is the column indices permuted into component-name order, computed once at
	// creation. The snapshot walks columns in this order so each entity's component list comes out
	// name-sorted with no per-snapshot sorting; wireCIDs are the matching component IDs, used to
	// look up each column's index in the snapshot's name table.
	wireOrder []int
	wireCIDs  []ComponentID
}

// newArchetype creates an archetype for the given component types.
func newArchetype(aid archetypeID, components bitmap.Bitmap, columns []abstractColumn) archetype {
	assert.That(components.Count() == len(columns), "mismatched number of columns and components")

	// Columns are laid out in ascending component-ID order (see worldState.newArchetype); the
	// snapshot wants them in name order. Both orders are fixed for the archetype's lifetime, so the
	// permutation is computed here, once, and the snapshot loop just reads it.
	wireOrder := make([]int, len(columns))
	for i := range wireOrder {
		wireOrder[i] = i
	}
	slices.SortFunc(wireOrder, func(x, y int) int {
		return strings.Compare(columns[x].name(), columns[y].name())
	})

	// cidsAsc[i] is the component ID of columns[i] (both follow ascending-ID order); wireCIDs
	// permutes it by wireOrder so wireCIDs[k] belongs to columns[wireOrder[k]].
	cidsAsc := make([]ComponentID, 0, len(columns))
	components.Range(func(cid uint32) {
		cidsAsc = append(cidsAsc, cid)
	})
	wireCIDs := make([]ComponentID, len(columns))
	for k, colIdx := range wireOrder {
		wireCIDs[k] = cidsAsc[colIdx]
	}

	return archetype{
		id:         aid,
		components: components,
		rows:       newSparseSet(),
		entities:   make([]EntityID, 0),
		columns:    columns,
		compCount:  len(columns),
		wireOrder:  wireOrder,
		wireCIDs:   wireCIDs,
	}
}

// exact returns true if the given components matches the archetype's exactly.
func (a *archetype) exact(components bitmap.Bitmap) bool {
	if a.compCount != components.Count() {
		return false
	}
	return a.contains(components)
}

// contains returns true if the archetype contains all of the components in the given components.
func (a *archetype) contains(components bitmap.Bitmap) bool {
	intersect := components.Clone(nil)
	intersect.And(a.components)
	return intersect.Count() == components.Count()
}

func (a *archetype) reset() {
	a.rows.clear()
	a.entities = a.entities[:0]
	for _, column := range a.columns {
		for column.len() > 0 {
			column.remove(column.len() - 1)
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Entity operations
// -------------------------------------------------------------------------------------------------

// newEntity adds the entity to the archetype. It initializes the entity's components with their
// zero values. This is done to ensure the length of each column matches the length of the entities
// slice.
func (a *archetype) newEntity(eid EntityID) {
	// Add to the entities slice.
	a.entities = append(a.entities, eid)

	// Extend the archetype's columns to make space for the new entity's components.
	for _, column := range a.columns {
		column.extend()
		assert.That(column.len() == len(a.entities), "column components length doesn't match entities")
	}

	// Map entity ID to its row.
	a.rows.set(eid, len(a.entities)-1)
}

// removeEntity removes an entity from the archetype. A remove swaps the last entity in the slice
// with the entity to remove, and returns the swapped entity ID. If the entity is the
// Expects the caller to check that the entity belongs to this archetype and is alive.
func (a *archetype) removeEntity(eid EntityID) {
	row, exists := a.rows.get(eid)
	assert.That(exists, "entity is not in archetype")

	lastIndex := len(a.entities) - 1

	// Swap the entity to remove with the last entity in the array.
	a.entities[row] = a.entities[lastIndex]
	// Truncate the array to remove the last entity.
	a.entities = a.entities[:lastIndex]

	// Remove the components of the entity.
	for _, column := range a.columns {
		column.remove(row)
		assert.That(column.len() == len(a.entities), "column components length doesn't match entities")
	}

	// Remove the entity from the row mapping.
	ok := a.rows.remove(eid)
	assert.That(ok, "entity isn't removed from sparse set")

	// If the entity is the last item in the slice, nothing is swapped so we can just return.
	if row == lastIndex {
		return
	}

	// Else, we update the swapped entity metadata to point to the correct row.
	movedID := a.entities[row]
	a.rows.set(movedID, row)
}

// moveEntity moves an entity from one archetype to another. It creates a new entity in the
// destination archetype, copies the component data from the current archetype, and removes the
// entity in the current archetype. Returns the swapped entity ID from the remove operation and the
// row in the destination archetype.
func (a *archetype) moveEntity(destination *archetype, eid EntityID) {
	// Normally I'd assert(src != dst) here, but since we have newEntityWithArchetype({}) is valid,
	// we'll just no-op instead of panic.
	if a == destination {
		return
	}

	row, exists := a.rows.get(eid)
	assert.That(exists, "entity is not in archetype")

	// Create a new entity in the destination archetype with the id.
	destination.newEntity(eid)
	newRow, exists := destination.rows.get(eid)
	assert.That(exists, "new entity isn't created in the destination archetype")

	// Move entity's components to the new archetype.
	for _, dst := range destination.columns {
		for _, src := range a.columns {
			if dst.name() == src.name() {
				value := src.getAbstract(row)
				dst.setAbstract(newRow, value)
			}
		}
	}

	// Remove the entity from the current archetype, which also updates the row mapping.
	a.removeEntity(eid)
}
