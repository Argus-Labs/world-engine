package immutable

import (
	"reflect"
	"sync"
)

// Storage that values share is never written, so an edit that retires part of an array cannot clear
// it: another value may still read it. Whatever the retired part holds then stays reachable for as
// long as the array does. For a type without pointers that is only the array's own bytes, but a
// string or a nested Slice keeps what it points at alive too, however large. So for such types the
// edits that would retire part of an array copy what survives instead, and a value built in bulk
// gives each chunk its own array.

// pointerTypes caches holdsPointers for arrays and structs, whose answer takes a walk over their
// elements or fields.
var pointerTypes sync.Map //nolint:gochecknoglobals // a cache of facts about types, the same for every caller

// holdsPointers reports whether a T can hold a pointer, which the garbage collector follows.
func holdsPointers[T any]() bool {
	t := reflect.TypeFor[T]()
	if k := t.Kind(); k != reflect.Array && k != reflect.Struct {
		return typeHoldsPointers(t)
	}

	if cached, ok := pointerTypes.Load(t); ok {
		if p, isBool := cached.(bool); isBool {
			return p
		}
	}

	p := typeHoldsPointers(t)
	pointerTypes.Store(t, p)

	return p
}

func typeHoldsPointers(t reflect.Type) bool {
	switch t.Kind() { //nolint:exhaustive // every other kind is or holds a pointer
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return false
	case reflect.Array:
		return t.Len() > 0 && typeHoldsPointers(t.Elem())
	case reflect.Struct:
		return fieldsHoldPointers(t)
	default:
		return true
	}
}

// fieldsHoldPointers reports whether any field of the struct type t can hold a pointer. It is apart
// from typeHoldsPointers because ranging over Fields allocates on entry to the function that does it,
// and the scalar kinds must not.
func fieldsHoldPointers(t reflect.Type) bool {
	for f := range t.Fields() {
		if typeHoldsPointers(f.Type) {
			return true
		}
	}

	return false
}
