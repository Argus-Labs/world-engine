package ecs

import (
	"encoding/json" //nolint:depguard // we need deterministic JSON encoding in column serialization

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/cardinal/v1"
	"github.com/rotisserie/eris"
)

// abstractColumn is an internal interface for generic column operations.
type abstractColumn interface {
	setAbstract(entity EntityID, value Component) error
	getAbstract(entity EntityID) (Component, bool)
	remove(entity EntityID)
	componentName() string
	serialize() (*cardinalv1.Column, error)
	deserialize(*cardinalv1.Column) error
}

var _ abstractColumn = &column[Component]{}

// column implements a sparse set data structure for efficient storage and lookup.
// It provides O(1) insertion, removal and lookup by using a sparse array to map entity IDs to
// dense storage indices.
//
// Note: column is not safe for concurrent use. Callers must ensure proper
// synchronization when accessing a column from multiple goroutines.
type column[T Component] struct {
	compName string // The name of the component stored in this column
	sparse   []int  // Maps entity IDs to an index in the dense array, -1 means not present

	// The denseEntityID and denseComponent array are parallel arrays.
	// It should always be updated together AND len(denseEntityID) == len(denseComponent).
	denseEntityID  []EntityID // Stores entity IDs
	denseComponent []T        // Stores component data
}

// newColumn creates a new sparse set with initial capacity. Returns a pointer to column because
// its methods use a pointer receiver.
func newColumn[T Component]() *column[T] {
	var zero T
	const initialCapacity = 16
	return &column[T]{
		compName:       zero.Name(),
		sparse:         make([]int, 0, initialCapacity), // Fixed size array
		denseEntityID:  make([]EntityID, 0, initialCapacity),
		denseComponent: make([]T, 0, initialCapacity),
	}
}

// newColumnConstructor returns a function that constructs a new column of type T.
func newColumnConstructor[T Component]() func() any {
	return func() any {
		return newColumn[T]()
	}
}

// -------------------------------------------------------------------------------------------------
// Column methods
// -------------------------------------------------------------------------------------------------

// set sets the component for the given entity. Used when the component is known to be of type T.
// This has better performance than SetAbstract since it avoids the type assertion.
func (c *column[T]) set(entity EntityID, value T) error {
	if entity > MaxEntityID {
		return eris.Errorf("entity ID %d exceeds max entity ID", entity)
	}

	if int(entity) >= len(c.sparse) {
		c.growSparse(entity)
	}

	// Fast path: entity already exists.
	if idx := c.sparse[entity]; idx != -1 {
		c.denseComponent[idx] = value
		return nil
	}

	// Entity does not exist, so we need to add it.
	newIdx := len(c.denseEntityID)
	c.sparse[entity] = newIdx
	c.appendDense(entity, value)

	return nil
}

// setAbstract sets the component for the given entity. Used when the component is an interface.
// This has worse performance than Set since it requires a type assertion.
func (c *column[T]) setAbstract(entity EntityID, value Component) error {
	if entity > MaxEntityID {
		return eris.Errorf("entity ID %d exceeds max entity ID", entity)
	}

	if int(entity) >= len(c.sparse) {
		c.growSparse(entity)
	}

	// Fast path: entity already exists.
	if idx := c.sparse[entity]; idx != -1 {
		c.denseComponent[idx] = value.(T) //nolint:errcheck // We know the type
		return nil
	}

	// Entity does not exist, so we need to add it.
	newIdx := len(c.denseEntityID)
	c.sparse[entity] = newIdx
	c.appendDense(entity, value.(T)) //nolint:errcheck // We know the type

	return nil
}

// get retrieves a value from the column. Returns the value and true if found, or the zero value
// and false if the entity doesn't exist in the column.
func (c *column[T]) get(entity EntityID) (T, bool) {
	if int(entity) >= len(c.sparse) {
		var zero T
		return zero, false // Entity doesn't exist in set
	}

	// Get dense array index from sparse array.
	idx := c.sparse[entity]
	if idx == -1 { // If the sparse array value is -1, then the entity doesn't exist
		var zero T
		return zero, false
	}

	// Return component data at dense array index.
	return c.denseComponent[idx], true
}

// getAbstract retrieves a value from the column. Returns the value and true if found, or the zero
// value and false if the entity doesn't exist in the column. This has worse performance than Get
// since it requires a type assertion.
func (c *column[T]) getAbstract(entity EntityID) (Component, bool) {
	v, ok := c.get(entity)
	if !ok {
		return nil, false
	}
	return v, true
}

// remove removes an element from the column. If the entity doesn't exist in the column, this is
// a no-op. Otherwise, it removes the entity and its associated data from the column.
// To maintain O(1) removal, it moves the last element into the removed element's position
// and updates the sparse array accordingly.
func (c *column[T]) remove(entity EntityID) {
	// Fast path: entity doesn't exist.
	// We can infer that the entity doesn't exist if the entity ID is greater than the length of the
	// sparse array. This is because the length of the sparse array is the maximum entity ID
	// that has been set.
	if int(entity) >= len(c.sparse) {
		return
	}

	// Fast path: entity doesn't exist.
	// We can infer that the entity doesn't exist if the sparse array maps the entity ID to -1. This
	// is because we set the sparse array to -1 as a placeholder for entities that don't exist.
	idx := c.sparse[entity]
	if idx == -1 || idx >= len(c.denseEntityID) {
		return
	}

	// Fast path: entity doesn't exist because the column is empty.
	lastIdx := len(c.denseEntityID) - 1
	if lastIdx < 0 {
		return
	}

	// Our remove method works by swapping out the last entity in the dense array with the entity to
	// remove to avoid having to shift all the elements down.

	// If the entity to remove is not the last entity in the dense array, then we swap it with
	// the last entity.
	if idx < lastIdx {
		c.sparse[c.denseEntityID[lastIdx]] = idx
		c.updateDense(idx, c.denseEntityID[lastIdx], c.denseComponent[lastIdx])
	}

	// Truncate the dense slices.
	// TODO: Should we truncate the sparse array as well?
	c.truncateDense(lastIdx)

	c.sparse[entity] = -1 // Mark entity as removed
}

// componentName returns the name of the component stored in this column.
func (c *column[T]) componentName() string {
	return c.compName
}

// -------------------------------------------------------------------------------------------------
// Dense array methods
//
// Prefer these methods over manually updating the dense arrays to ensure that the
// invariant len(denseEntityID) == len(denseComponent) is maintained.
// -------------------------------------------------------------------------------------------------

// appendDense appends an entity and its corresponding component to the dense arrays.
func (c *column[T]) appendDense(entity EntityID, value T) {
	c.denseEntityID = append(c.denseEntityID, entity)
	c.denseComponent = append(c.denseComponent, value)
}

// updateDense updates the dense arrays at the given index.
func (c *column[T]) updateDense(idx int, entity EntityID, value T) {
	c.denseEntityID[idx] = entity
	c.denseComponent[idx] = value
}

// truncateDense truncates the dense arrays at the given index.
func (c *column[T]) truncateDense(idx int) {
	c.denseEntityID = c.denseEntityID[:idx]
	c.denseComponent = c.denseComponent[:idx]
}

// growSparse grows the sparse array to accommodate the given ID. We expect id <= MaxEntityID.
func (c *column[T]) growSparse(id EntityID) {
	// Only grow to exactly what's needed
	// TODO: Could probably optimize this by growing by a factor of 2
	newCap := id + 1
	newSparse := make([]int, newCap)
	for i := range newSparse {
		newSparse[i] = -1
	}
	copy(newSparse, c.sparse)
	c.sparse = newSparse
}

func toAbstractColumn(v any) abstractColumn {
	col, ok := v.(abstractColumn)
	assert.That(ok, "column is of type %T, expected abstractColumn", v)
	return col
}

// -------------------------------------------------------------------------------------------------
// Serialization methods
// -------------------------------------------------------------------------------------------------

// serialize converts the column to a protobuf message for serialization.
func (c *column[T]) serialize() (*cardinalv1.Column, error) {
	sparse := make([]int64, len(c.sparse))
	for i, val := range c.sparse {
		sparse[i] = int64(val)
	}

	denseEntityIDs := make([]uint32, len(c.denseEntityID))
	for i, entityID := range c.denseEntityID {
		denseEntityIDs[i] = uint32(entityID)
	}

	// Serialize each component to JSON bytes using deterministic encoding/json.
	denseComponentData := make([][]byte, len(c.denseComponent))
	for i, component := range c.denseComponent {
		data, err := json.Marshal(component)
		if err != nil {
			return nil, eris.Wrapf(err, "failed to serialize component at index %d", i)
		}
		denseComponentData[i] = data
	}

	return &cardinalv1.Column{
		ComponentName:      c.compName,
		Sparse:             sparse,
		DenseEntityIds:     denseEntityIDs,
		DenseComponentData: denseComponentData,
	}, nil
}

// deserialize populates the column from a protobuf message.
func (c *column[T]) deserialize(pb *cardinalv1.Column) error {
	if pb.GetComponentName() != c.compName {
		return eris.Errorf("component name mismatch: expected %s, got %s", c.compName, pb.GetComponentName())
	}

	c.sparse = make([]int, len(pb.GetSparse()))
	for i, val := range pb.GetSparse() {
		c.sparse[i] = int(val)
	}

	c.denseEntityID = make([]EntityID, len(pb.GetDenseEntityIds()))
	for i, entityID := range pb.GetDenseEntityIds() {
		c.denseEntityID[i] = EntityID(entityID)
	}

	// Deserialize component data from JSON bytes.
	c.denseComponent = make([]T, len(pb.GetDenseComponentData()))
	for i, data := range pb.GetDenseComponentData() {
		var component T
		if err := json.Unmarshal(data, &component); err != nil {
			return eris.Wrapf(err, "failed to deserialize component at index %d", i)
		}
		c.denseComponent[i] = component
	}

	return nil
}

// -------------------------------------------------------------------------------------------------
// Test helpers
// -------------------------------------------------------------------------------------------------

// clear removes all elements from the sparse set while preserving capacity.
func (c *column[T]) clear() {
	c.sparse = c.sparse[:0]
	c.truncateDense(0)
}

// len returns the number of entities in the column.
func (c *column[T]) len() int {
	return len(c.denseEntityID)
}

// contains checks if an entity exists in the column.
func (c *column[T]) contains(entity EntityID) bool {
	return int(entity) < len(c.sparse) && c.sparse[entity] != -1
}
