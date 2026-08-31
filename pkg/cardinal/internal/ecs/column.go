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

	rowWireSize(row int) int
	appendRowWire(b []byte, row int) []byte
	decodeRow(row int, data []byte) error

	//TODO: removed when maps and slices are gone
	stagedRowWireSize(row int) int
}

var _ abstractColumn = &column[Component]{}

// column stores the component data of entities in an archetype. The length of the components slice
// must match the length of the entities slice in the archetype.
type column[T Component] struct {
	compName   string // The name of the component stored in this column
	components []T    // Array containing the component data

	//TODO: removed when maps and slices are gone
	direct    bool     // Whether T carries the generated zero-alloc encoders.
	wireCache [][]byte // The per-row staging area for types that don't (see rowWireSize).
}

const columnCapacity = 16

// newColumn creates a new column with the specified type.
func newColumn[T Component]() column[T] {
	var zero T
	_, direct := any((*T)(nil)).(directWire) // Probs if contains pointer types, to be removed.
	return column[T]{
		compName:   zero.Name(),
		components: make([]T, 0, columnCapacity),
		direct:     direct,
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

// directWire is the generated fast encoding path: SizeWire reports the exact encoded size and
// AppendWire writes exactly that many bytes into the caller's buffer, both without allocating.
// The generator emits the pair only for value-shaped types (no slices, maps, or JSON-fallback
// fields), so its presence is probed once per column and everything else takes the MarshalWire
// fallback below.
// TODO: remove when maps and slices removed.
type directWire interface {
	SizeWire() int
	AppendWire([]byte) []byte
}

// rowWireSize returns the encoded size of one row and stages what appendRowWire needs.
//
// Direct components are pure arithmetic. Fallback components must be encoded to be sized, so the
// bytes are kept in wireCache (reused across snapshots, grow-only) and appendRowWire copies them
// out — one allocation per row inside MarshalWire, the same cost the old path paid, gone entirely
// once the component becomes value-shaped and picks up the generated encoders.
//
// TODO: delete the fallback path once every component is guaranteed direct. That needs two things
// to land first: plugins regenerated with the SizeWire/AppendWire generator, and the slice/map
// removal from component schemas — ineligible types are the only reason a generated component
// lacks the direct encoders. Then SizeWire/AppendWire move onto the Component contract, the
// directWire interface and its runtime probe disappear, and this whole function becomes:
//
//	func (c *column[T]) rowWireSize(row int) int {
//		assert.That(row < len(c.components), "component doesn't exist")
//		return c.components[row].SizeWire()
//	}
//
// Going with it: the c.direct field and probe, wireCache, stagedRowWireSize (identical to
// rowWireSize once nothing stages), the marshal-failure assert, and the pointer-boxing dance —
// which only exists to keep the runtime type assertion allocation-free.
func (c *column[T]) rowWireSize(row int) int {
	assert.That(row < len(c.components), "component doesn't exist")
	if c.direct {
		dw, ok := any(&c.components[row]).(directWire)
		assert.That(ok, "direct column element lost its wire encoders")
		return dw.SizeWire()
	}

	// Nothing recoverable reaches here: a component either cannot encode at all (a bug in its
	// generated code) or holds a value protobuf rejects, such as a string with invalid UTF-8. Both
	// repeat on every tick, so the alternative to crashing is a world that silently stops
	// snapshotting behind a warning log.
	data, err := c.components[row].MarshalWire()
	assert.That(err == nil, "failed to serialize component %q at row %d: %v", c.compName, row, err)
	for len(c.wireCache) <= row {
		c.wireCache = append(c.wireCache, nil)
	}
	c.wireCache[row] = data
	return len(data)
}

// stagedRowWireSize returns the size rowWireSize staged, without re-encoding: the append pass
// needs each payload's length prefix a second time, and re-marshaling a fallback component there
// would both allocate and race the size the first pass reported.
// TODO: removed with slices and maps gone
func (c *column[T]) stagedRowWireSize(row int) int {
	assert.That(row < len(c.components), "component doesn't exist")
	if c.direct {
		dw, ok := any(&c.components[row]).(directWire)
		assert.That(ok, "direct column element lost its wire encoders")
		return dw.SizeWire()
	}
	assert.That(row < len(c.wireCache) && c.wireCache[row] != nil,
		"stagedRowWireSize called without a staging rowWireSize call")
	return len(c.wireCache[row])
}

// appendRowWire writes one row's encoded bytes. The caller must have called rowWireSize for this
// row since the last world mutation — that call either proved the direct path or staged the
// fallback bytes this one copies.
//
// TODO: with the fallback gone (see rowWireSize), the staging precondition goes with it and this
// becomes:
//
//	func (c *column[T]) appendRowWire(b []byte, row int) []byte {
//		assert.That(row < len(c.components), "component doesn't exist")
//		return c.components[row].AppendWire(b)
//	}
func (c *column[T]) appendRowWire(b []byte, row int) []byte {
	assert.That(row < len(c.components), "component doesn't exist")
	if c.direct {
		dw, ok := any(&c.components[row]).(directWire)
		assert.That(ok, "direct column element lost its wire encoders")
		return dw.AppendWire(b)
	}

	assert.That(row < len(c.wireCache) && c.wireCache[row] != nil,
		"appendRowWire called without a staging rowWireSize call")
	return append(b, c.wireCache[row]...)
}

// decodeRow deserializes one component payload into an existing row.
func (c *column[T]) decodeRow(row int, data []byte) error {
	assert.That(row < len(c.components), "component doesn't exist")

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
