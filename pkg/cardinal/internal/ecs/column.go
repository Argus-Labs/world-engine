package ecs

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
)

// columnFactory is a function that creates a new abstractColumn instance.
type columnFactory func() abstractColumn

// abstractColumn is an internal interface for generic column operations.
type abstractColumn interface {
	len() int
	name() string
	extend()

	setAbstract(row int, component Component)
	getAbstract(row int) Component
	remove(row int)

	encodeRows() ([][]byte, error)
	decodeRow(row int, data []byte) error
}

var _ abstractColumn = &column[Component]{}

// column stores the component data of entities in an archetype. The length of the components slice
// must match the length of the entities slice in the archetype.
type column[T Component] struct {
	compName   string // The name of the component stored in this column
	components []T    // Array containing the component data
}

const columnCapacity = 16

// newColumn creates a new column with the specified type.
func newColumn[T Component]() column[T] {
	var zero T
	return column[T]{
		compName:   zero.Name(),
		components: make([]T, 0, columnCapacity),
	}
}

// newColumnFactory returns a function that constructs a new column of type T.
func newColumnFactory[T Component]() columnFactory {
	return func() abstractColumn {
		col := newColumn[T]()
		return &col
	}
}

// len returns the length of the components slice.
func (c *column[T]) len() int {
	return len(c.components)
}

// name returns the name of the component type.
func (c *column[T]) name() string {
	return c.compName
}

// extend adds a new row to the components slice and initializes them with the zero value.
func (c *column[T]) extend() {
	// Double the capacity when the capacity is reached.
	if len(c.components) == cap(c.components) {
		newCap := cap(c.components) * 2
		newComponents := make([]T, len(c.components), newCap)
		copy(newComponents, c.components)
		c.components = newComponents
	}

	var zero T
	c.components = append(c.components, zero)
}

// set sets the component in a given row. A row corresponds to a single entity. Whenever possible
// prefer this method over setAbstract since it avoids the type assertion and avoids boxing the
// component data, which does allocations.
func (c *column[T]) set(row int, component T) {
	assert.That(row < len(c.components), "column isn't extended when entity is created")
	c.components[row] = component
}

// setAbstract sets the component in a given row. A row corresponds to a single entity. Use this
// method only when you don't know the concrete type of the component.
func (c *column[T]) setAbstract(row int, component Component) {
	concrete, ok := component.(T)
	assert.That(ok, "tried to set the wrong component type")
	c.set(row, concrete)
}

// get gets the value from a given row. A row corresponds to a single entity. Expects the caller
// to make sure the row is inside the column. Whenever possible prefer this method over getAbstract
// since it avoids the type assertion and avoids boxing the component data, which does allocations.
func (c *column[T]) get(row int) T {
	assert.That(row < len(c.components), "component doesn't exist")
	return c.components[row]
}

// getAbstract gets the value from a given row. A row corresponds to a single entity. Expects the
// caller to make sure the row is inside the column. Use this method only when you don't know the
// concrete type of the component.
func (c *column[T]) getAbstract(row int) Component {
	return c.get(row)
}

// remove removes a given row. A row corresponds to a single entity. Expects the caller to make sure
// the row is inside the column. A remove swaps the last value in the slice with the row to remove.
func (c *column[T]) remove(row int) {
	assert.That(row < len(c.components), "tried to remove component that doesn't exist")

	lastIndex := len(c.components) - 1

	// Removing a component is the same as moving the entity to another archetype.
	// Swap the component to remove with the last component in the array.
	c.components[row] = c.components[lastIndex]
	// Truncate the array to remove the last component.
	c.components = c.components[:lastIndex]
}

// encodeRows serializes every component in the column, in row order. Each component encodes through
// its generated MarshalWire (proto) — no msgpack. T is a Component (embeds schema.Serializable), so
// MarshalWire is guaranteed by the type; an ungenerated component wouldn't satisfy the constraint
// and wouldn't compile.
func (c *column[T]) encodeRows() ([][]byte, error) {
	payloads := make([][]byte, len(c.components))
	for i, component := range c.components {
		data, err := component.MarshalWire()
		if err != nil {
			return nil, eris.Wrapf(err, "failed to serialize component at row %d", i)
		}
		payloads[i] = data
	}
	return payloads, nil
}

// decodeRow deserializes one component payload into an existing row. Each component decodes through
// its generated UnmarshalWire (proto) — no msgpack (see encodeRows).
func (c *column[T]) decodeRow(row int, data []byte) error {
	var zero T
	decoded, err := zero.UnmarshalWire(data)
	if err != nil {
		return eris.Wrapf(err, "failed to deserialize component %q", c.compName)
	}
	typed, ok := decoded.(T)
	if !ok {
		return eris.Errorf("component %q decoded to unexpected type %T", c.compName, decoded)
	}
	c.components[row] = typed
	return nil
}
