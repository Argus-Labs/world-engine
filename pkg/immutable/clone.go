package immutable

import (
	"reflect"
	"sync"
)

// CloneFunc returns a function that gives every Slice reachable from a T its own array, so the T no
// longer shares one with the value it was copied from. It returns nil when T holds no Slice, so a
// caller can skip the work entirely; Cardinal builds one per component type at registration and
// calls it on every Get and Set.
//
// It reaches Slices in struct fields (exported or not), fixed arrays, the elements of other Slices
// and the values of Maps. It does not follow pointers, interfaces, or Go slices and maps, which a
// component cannot hold.
func CloneFunc[T any]() func(*T) {
	clone := clonerOf(reflect.TypeFor[T]())
	if clone == nil {
		return nil
	}
	return func(p *T) { clone(reflect.ValueOf(p).Elem()) }
}

// Clone returns a copy of v that shares no Slice array with it. When T is an interface type, it
// copies the value the interface holds. It looks T up on every call; for a type copied often, keep
// the function CloneFunc returns instead.
func Clone[T any](v T) T {
	t := reflect.TypeFor[T]()
	if t.Kind() != reflect.Interface {
		if clone := clonerOf(t); clone != nil {
			clone(reflect.ValueOf(&v).Elem())
		}
		return v
	}
	held := reflect.ValueOf(v)
	if !held.IsValid() {
		return v
	}
	clone := clonerOf(held.Type())
	if clone == nil {
		return v
	}
	cp := reflect.New(held.Type()).Elem()
	cp.Set(held)
	clone(cp)
	return cp.Interface().(T) //nolint:errcheck,forcetypeassert // cp holds v's dynamic type, which implements T.
}

// deepCloner is implemented by *Slice and *Map: cloneDeep gives the value its own arrays, wherever
// a shared one could be edited.
type deepCloner interface{ cloneDeep() }

func (s *Slice[T]) cloneDeep() {
	if len(s.items) == 0 {
		s.items = nil
		return
	}
	items := make([]T, len(s.items))
	copy(items, s.items)
	if clone := clonerOf(reflect.TypeFor[T]()); clone != nil {
		for i := range items {
			clone(reflect.ValueOf(&items[i]).Elem())
		}
	}
	s.items = items
}

// cloneDeep copies the values only when they hold a Slice. A Map's own arrays are never written once
// a Map holds them, so sharing them is safe; a Slice inside a value is edited in place, so it is not.
func (m *Map[K, V]) cloneDeep() {
	clone := clonerOf(reflect.TypeFor[V]())
	if clone == nil || len(m.vals) == 0 {
		return
	}
	vals := make([]V, len(m.vals))
	copy(vals, m.vals)
	for i := range vals {
		clone(reflect.ValueOf(&vals[i]).Elem())
	}
	m.vals = vals
}

// cloner gives every Slice reachable from v its own array. v is addressable and of the type the
// cloner was built for.
type cloner func(v reflect.Value)

// cloners caches one cloner per type, [reflect.Type] to cloner. A nil cloner means the type holds no
// Slice. It is global because Clone, called on every event and command, has no other place to keep
// one, and building a cloner walks the whole type.
var cloners sync.Map //nolint:gochecknoglobals // A per-type cache of pure functions of the type.

func clonerOf(t reflect.Type) cloner {
	if c, ok := cloners.Load(t); ok {
		return c.(cloner) //nolint:errcheck,forcetypeassert // Only cloners are stored.
	}
	c := buildCloner(t)
	cloners.Store(t, c)
	return c
}

func buildCloner(t reflect.Type) cloner {
	if !holdsSlice(t, map[reflect.Type]bool{}) {
		return nil
	}
	if reflect.PointerTo(t).Implements(reflect.TypeFor[deepCloner]()) {
		return func(v reflect.Value) {
			// NewAt rather than v.Addr().Interface(), which panics when v was reached through an
			// unexported field. The pointer is v's own address, typed as v's own type.
			dc := reflect.NewAt(t, v.Addr().UnsafePointer()).Interface().(deepCloner) //nolint:errcheck,forcetypeassert,lll // Implements checked it.
			dc.cloneDeep()
		}
	}
	switch t.Kind() { //nolint:exhaustive // holdsSlice is true only for structs, arrays, Slices and Maps.
	case reflect.Struct:
		type field struct {
			index int
			clone cloner
		}
		var fields []field
		for i := range t.NumField() {
			// No cycle: a struct cannot hold itself except through a Slice or Map, and those stop
			// above without building a cloner for their elements.
			if clone := clonerOf(t.Field(i).Type); clone != nil {
				fields = append(fields, field{index: i, clone: clone})
			}
		}
		return func(v reflect.Value) {
			for _, f := range fields {
				f.clone(v.Field(f.index))
			}
		}
	case reflect.Array:
		elem := clonerOf(t.Elem())
		return func(v reflect.Value) {
			for i := range v.Len() {
				elem(v.Index(i))
			}
		}
	}
	return nil
}

// holdsSlice reports whether a Slice is reachable from t. visiting holds the types on the current
// path: a type that reaches itself, such as a tree of Maps, holds a Slice only if some other path
// from it does.
func holdsSlice(t reflect.Type, visiting map[reflect.Type]bool) bool {
	if visiting[t] {
		return false
	}
	if _, ok := SliceElem(t); ok {
		return true
	}
	visiting[t] = true
	defer delete(visiting, t)
	if _, value, ok := MapTypes(t); ok {
		return holdsSlice(value, visiting)
	}
	switch t.Kind() { //nolint:exhaustive // No other kind a component may hold contains a Slice.
	case reflect.Struct:
		for f := range t.Fields() {
			if holdsSlice(f.Type, visiting) {
				return true
			}
		}
	case reflect.Array:
		return t.Len() > 0 && holdsSlice(t.Elem(), visiting)
	}
	return false
}
