package immutable

import (
	"reflect"
	"sync"
)

// CloneFunc returns a function that gives every Slice and Map reachable from a T its own storage, so
// the T no longer shares any with the value it was copied from. It returns nil when T holds neither,
// so a caller can skip the work entirely; Cardinal builds one per component type at registration and
// calls it on every Get and Set.
//
// It reaches Slices and Maps in struct fields (exported or not), fixed arrays, and inside other
// Slices and Maps. It does not follow pointers, interfaces, or Go slices and maps, which a component
// cannot hold.
func CloneFunc[T any]() func(*T) {
	clone := clonerOf(reflect.TypeFor[T]())
	if clone == nil {
		return nil
	}
	return func(p *T) { clone(reflect.ValueOf(p).Elem()) }
}

// Clone returns a copy of v that shares no Slice or Map storage with it. When T is an interface type,
// it copies the value the interface holds. It looks T up on every call; for a type copied often,
// keep the function CloneFunc returns instead.
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

// deepCloner is implemented by *Slice and *Map: cloneDeep gives the value its own arrays, and does the
// same for every Slice and Map inside its elements.
type deepCloner interface{ cloneDeep() }

var (
	_ deepCloner = (*Slice[int])(nil)
	_ deepCloner = (*Map[int, int])(nil)
)

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

func (m *Map[K, V]) cloneDeep() {
	if len(m.keys) == 0 {
		m.keys, m.vals = nil, nil
		return
	}
	keys := make([]K, len(m.keys))
	copy(keys, m.keys)
	vals := make([]V, len(m.vals))
	copy(vals, m.vals)
	if clone := clonerOf(reflect.TypeFor[V]()); clone != nil {
		for i := range vals {
			clone(reflect.ValueOf(&vals[i]).Elem())
		}
	}
	m.keys, m.vals = keys, vals
}

// cloner gives every Slice and Map reachable from v its own storage. v is addressable and of the type
// the cloner was built for.
type cloner func(v reflect.Value)

// cloners caches one cloner per type, [reflect.Type] to cloner. A nil cloner means the type holds no
// Slice or Map. It is global because Clone, called on every event and command, has no other place to
// keep one, and building a cloner walks the whole type.
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
	if !holdsCollection(t) {
		return nil
	}
	// t itself, not its method set: a struct that embeds a Slice or Map gets cloneDeep promoted, and
	// treating it as one would skip its other fields.
	_, isSlice := SliceElem(t)
	if _, _, isMap := MapTypes(t); isSlice || isMap {
		return func(v reflect.Value) {
			// NewAt rather than v.Addr().Interface(), which panics when v was reached through an
			// unexported field. The pointer is v's own address, typed as v's own type.
			dc := reflect.NewAt(t, v.Addr().UnsafePointer()).Interface().(deepCloner) //nolint:errcheck,forcetypeassert,lll // *Slice and *Map implement it.
			dc.cloneDeep()
		}
	}
	switch t.Kind() { //nolint:exhaustive // holdsCollection is true only for structs, arrays, Slices and Maps.
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

// holdsCollection reports whether a Slice or Map is reachable from t. It needs no cycle check: a type
// can reach itself only through a Slice or Map, and the walk stops there.
func holdsCollection(t reflect.Type) bool {
	if _, ok := SliceElem(t); ok {
		return true
	}
	if _, _, ok := MapTypes(t); ok {
		return true
	}
	switch t.Kind() { //nolint:exhaustive // No other kind a component may hold contains a Slice or Map.
	case reflect.Struct:
		for f := range t.Fields() {
			if holdsCollection(f.Type) {
				return true
			}
		}
	case reflect.Array:
		return t.Len() > 0 && holdsCollection(t.Elem())
	}
	return false
}
