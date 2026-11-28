package immutable

import (
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestHoldsPointers(t *testing.T) {
	t.Parallel()

	type id uint64

	type flat struct {
		A int64
		B [4]float32
		C bool
		D id
	}

	type nested struct {
		F flat
		S string
	}

	assert.False(t, holdsPointers[int](), "int")
	assert.False(t, holdsPointers[id](), "a named integer")
	assert.False(t, holdsPointers[complex128](), "complex128")
	assert.False(t, holdsPointers[[8]uint8](), "an array of bytes")
	assert.False(t, holdsPointers[[0]string](), "an empty array")
	assert.False(t, holdsPointers[flat](), "a struct of scalars")
	assert.False(t, holdsPointers[struct{}](), "an empty struct")
	assert.False(t, holdsPointers[entry[int, flat]](), "a Map entry of scalars")

	assert.True(t, holdsPointers[string](), "string")
	assert.True(t, holdsPointers[[2]string](), "an array of strings")
	assert.True(t, holdsPointers[nested](), "a struct holding a string")
	assert.True(t, holdsPointers[time.Time](), "time.Time, which points at its Location")
	assert.True(t, holdsPointers[Slice[int]](), "a nested Slice")
	assert.True(t, holdsPointers[Map[int, int]](), "a nested Map")
	assert.True(t, holdsPointers[entry[string, int]](), "a Map entry with a string key")
	assert.True(t, holdsPointers[unsafe.Pointer](), "unsafe.Pointer")
	assert.True(t, holdsPointers[any](), "an interface")
	// Arrays and structs are cached; holdsPointers[nested] ran above.
	cached, ok := pointerTypes.Load(reflect.TypeFor[nested]())
	assert.True(t, ok, "a struct's answer is cached")
	assert.Equal(t, true, cached, "the cached answer")
	assert.True(t, holdsPointers[nested](), "a struct holding a string, from the cache")
}
