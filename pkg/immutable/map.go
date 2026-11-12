package immutable

import (
	"bytes"
	"cmp"
	"iter"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"github.com/vmihailenco/msgpack/v5"
)

// Key is the set of types a Map may be keyed by: integers, strings, and named types of them, such as
// `type PlayerID uint64`. Their < is a total order and the same on every platform, which is what
// keeps a Map's layout and encoding deterministic. Floats are left out because NaN breaks sorting,
// and structs because they have no < at all.
type Key interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~string
}

// Map is a sorted map with private storage, for component fields that look values up by key. It is
// to a Go map what Slice is to a []T: a copy from Get shares nothing that can change, so editing it
// never reaches the world without Set.
//
// Entries are kept sorted by key, never hashed. Iteration and encoding run in key order, so the same
// entries give the same order and the same bytes on every run, machine and build, however they were
// inserted. "First" means smallest key; if insertion order matters, store it in the value.
//
// Derivations never write to the receiver's arrays. With, Without and Update each return a new Map,
// so the column, the caller's copy and any snapshot keep what they held:
//
//	c := npc.Get[NPCMemory]()
//	mem, _ := c.Players.Get(pid)
//	mem.Affinity += 5 // edits the copy only
//	c.Players = c.Players.With(pid, mem)
//	npc.Set(c)
//
// Each derivation costs one copy. For several edits at once, use Edit, which copies once and hands
// the result over with Done.
//
// The zero value is an empty Map, and every empty Map is the zero value.
//
// The value type must be value-safe, the same rule Slice elements follow. Map does not check this;
// registering a component that holds such a Map fails.
type Map[K Key, V any] struct {
	keys []K // sorted ascending, no duplicates
	vals []V // vals[i] belongs to keys[i]
}

// -------------------------------------------------------------------------------------------------
// Construction
// -------------------------------------------------------------------------------------------------

// MapFrom returns a Map holding every pair in seq. When a key repeats, its last value wins. The
// result depends only on the pairs, not on the order seq yields them, so MapFrom(maps.All(m)) is
// deterministic even though ranging over m is not.
func MapFrom[K Key, V any](seq iter.Seq2[K, V]) Map[K, V] {
	type entry struct {
		k K
		v V
	}
	var entries []entry
	for k, v := range seq {
		entries = append(entries, entry{k, v})
	}
	if len(entries) == 0 {
		return Map[K, V]{}
	}
	// Stable, so of the entries sharing a key the one seq yielded last is still last.
	slices.SortStableFunc(entries, func(a, b entry) int { return cmp.Compare(a.k, b.k) })
	keys := make([]K, 0, len(entries))
	vals := make([]V, 0, len(entries))
	for i, e := range entries {
		if i+1 < len(entries) && entries[i+1].k == e.k {
			continue
		}
		keys = append(keys, e.k)
		vals = append(vals, e.v)
	}
	return Map[K, V]{keys: keys, vals: vals}
}

// -------------------------------------------------------------------------------------------------
// Reading
// -------------------------------------------------------------------------------------------------

// Len returns the number of entries.
func (m Map[K, V]) Len() int {
	return len(m.keys)
}

// Get returns a copy of the value stored under k, and whether k is present. A missing key yields the
// zero value.
func (m Map[K, V]) Get(k K) (V, bool) {
	if i, ok := slices.BinarySearch(m.keys, k); ok {
		return m.vals[i], true
	}
	var zero V
	return zero, false
}

// Has reports whether k is present.
func (m Map[K, V]) Has(k K) bool {
	_, ok := slices.BinarySearch(m.keys, k)
	return ok
}

// All returns an iterator over key and value, in ascending key order. Values are yielded by value.
func (m Map[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for i, k := range m.keys {
			if !yield(k, m.vals[i]) {
				return
			}
		}
	}
}

// Keys returns an iterator over the keys, in ascending order.
func (m Map[K, V]) Keys() iter.Seq[K] {
	return slices.Values(m.keys)
}

// Values returns an iterator over the values, in ascending key order.
func (m Map[K, V]) Values() iter.Seq[V] {
	return slices.Values(m.vals)
}

// -------------------------------------------------------------------------------------------------
// Deriving
// -------------------------------------------------------------------------------------------------

// With returns a Map with v stored under k, replacing any value already there. Replacing a value
// copies only the values; adding a key copies both arrays.
//
// NOTE: Requires component set if used.
func (m Map[K, V]) With(k K, v V) Map[K, V] {
	i, ok := slices.BinarySearch(m.keys, k)
	if ok {
		vals := slices.Clone(m.vals)
		vals[i] = v
		return Map[K, V]{keys: m.keys, vals: vals}
	}
	return Map[K, V]{keys: insertAt(m.keys, i, k), vals: insertAt(m.vals, i, v)}
}

// Without returns a Map with k removed. When k is absent it returns the receiver, which costs
// nothing.
//
// NOTE: Requires component set if used.
func (m Map[K, V]) Without(k K) Map[K, V] {
	i, ok := slices.BinarySearch(m.keys, k)
	if !ok {
		return m
	}
	if len(m.keys) == 1 {
		return Map[K, V]{}
	}
	return Map[K, V]{keys: removeAt(m.keys, i), vals: removeAt(m.vals, i)}
}

// Update returns a Map with the value under k replaced by f of the current one. f receives the zero
// value when k is absent, so counters and accumulators need no separate Has check.
//
// NOTE: Requires component set if used.
func (m Map[K, V]) Update(k K, f func(V) V) Map[K, V] {
	v, _ := m.Get(k)
	return m.With(k, f(v))
}

// insertAt returns a new array holding s with v inserted at index i.
func insertAt[T any](s []T, i int, v T) []T {
	out := make([]T, 0, len(s)+1)
	out = append(out, s[:i]...)
	out = append(out, v)
	return append(out, s[i:]...)
}

// removeAt returns a new array holding s without the element at index i.
func removeAt[T any](s []T, i int) []T {
	out := make([]T, 0, len(s)-1)
	out = append(out, s[:i]...)
	return append(out, s[i+1:]...)
}

// -------------------------------------------------------------------------------------------------
// Comparing
// -------------------------------------------------------------------------------------------------

// EqualFunc reports whether both maps hold the same keys and eq holds for the values under each.
func (m Map[K, V]) EqualFunc(other Map[K, V], eq func(a, b V) bool) bool {
	return slices.Equal(m.keys, other.keys) && slices.EqualFunc(m.vals, other.vals, eq)
}

// MapEqual reports whether a and b hold the same entries. It is a function rather than a method
// because a method cannot narrow V to comparable; use EqualFunc for other value types.
func MapEqual[K Key, V comparable](a, b Map[K, V]) bool {
	return slices.Equal(a.keys, b.keys) && slices.Equal(a.vals, b.vals)
}

// -------------------------------------------------------------------------------------------------
// Serialization and Reflection
// -------------------------------------------------------------------------------------------------
// A Map encodes as a map, in JSON and MessagePack alike, with entries in key order so equal Maps
// encode to equal bytes. Both decoders accept entries in any order and sort them once, so a redeploy
// that changes a key's type, say int32 to uint32, still loads the old bytes. A repeated key is an
// error: there is no right value to keep. As with Slice, the JSON methods exist because otherwise the
// encoder sees a struct of unexported fields and writes `{}`.

// MarshalJSON encodes the Map as a JSON object in key order. JSON keys are strings, so an integer key
// is written in decimal, as encoding/json does for a Go map.
func (m Map[K, V]) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(keyText(k))
		if err != nil {
			return nil, eris.Wrap(err, "failed to encode immutable map key")
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := json.Marshal(m.vals[i])
		if err != nil {
			return nil, eris.Wrap(err, "failed to encode immutable map value")
		}
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON replaces the Map with the object in data. `null` and `{}` yield the zero value. An
// integer key must be written in canonical decimal, as MarshalJSON writes it. The Map is replaced
// only when the whole object decodes.
func (m *Map[K, V]) UnmarshalJSON(data []byte) error {
	if string(bytes.TrimSpace(data)) == "null" {
		*m = Map[K, V]{}
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return eris.New("immutable map: expected a JSON object")
	}
	var keys []K
	var vals []V
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return eris.Wrap(err, "failed to decode immutable map key")
		}
		text, ok := tok.(string)
		if !ok {
			return eris.Errorf("immutable map: expected a string key, got %v", tok)
		}
		k, err := parseKey[K](text)
		if err != nil {
			return err
		}
		var v V
		if err := dec.Decode(&v); err != nil {
			return eris.Wrapf(err, "failed to decode immutable map value for key %q", text)
		}
		keys = append(keys, k)
		vals = append(vals, v)
	}
	if _, err := dec.Token(); err != nil {
		return eris.Wrap(err, "failed to decode immutable map end")
	}
	decoded, err := sortedMap(keys, vals)
	if err != nil {
		return err
	}
	*m = decoded
	return nil
}

// EncodeMsgpack encodes the Map as a MessagePack map in key order.
func (m Map[K, V]) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeMapLen(len(m.keys)); err != nil {
		return eris.Wrap(err, "failed to encode immutable map length")
	}
	for i, k := range m.keys {
		if err := enc.Encode(k); err != nil {
			return eris.Wrap(err, "failed to encode immutable map key")
		}
		if err := enc.Encode(m.vals[i]); err != nil {
			return eris.Wrap(err, "failed to encode immutable map value")
		}
	}
	return nil
}

// DecodeMsgpack replaces the Map only after every entry has decoded successfully. Arrays grow as
// entries arrive rather than from the claimed length, so a forged length cannot force a large
// allocation. nil and an empty map yield the zero value.
func (m *Map[K, V]) DecodeMsgpack(dec *msgpack.Decoder) error {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return eris.Wrap(err, "failed to decode immutable map length")
	}
	var keys []K
	var vals []V
	for range n {
		var k K
		if err := dec.Decode(&k); err != nil {
			return eris.Wrap(err, "failed to decode immutable map key")
		}
		var v V
		if err := dec.Decode(&v); err != nil {
			return eris.Wrap(err, "failed to decode immutable map value")
		}
		keys = append(keys, k)
		vals = append(vals, v)
	}
	decoded, err := sortedMap(keys, vals)
	if err != nil {
		return err
	}
	*m = decoded
	return nil
}

// IsZero lets MessagePack's omitempty distinguish populated Maps from empty ones.
func (m Map[K, V]) IsZero() bool { return len(m.keys) == 0 }

// MapTypes reports whether t is an instantiation of Map and, if so, returns its key and value types.
// Like SliceElem, it lets reflection-based tooling recognise a Map without depending on its layout.
func MapTypes(t reflect.Type) (reflect.Type, reflect.Type, bool) {
	if t == nil || t.Kind() != reflect.Struct || !strings.HasPrefix(t.Name(), "Map[") {
		return nil, nil, false
	}
	if t.PkgPath() != reflect.TypeFor[Map[int, struct{}]]().PkgPath() {
		return nil, nil, false
	}
	keys, okKeys := t.FieldByName("keys")
	vals, okVals := t.FieldByName("vals")
	if !okKeys || !okVals || keys.Type.Kind() != reflect.Slice || vals.Type.Kind() != reflect.Slice {
		return nil, nil, false
	}
	return keys.Type.Elem(), vals.Type.Elem(), true
}

// sortedMap builds a Map from decoded entries in whatever order they arrived. Input that is already
// sorted, which is everything this package encoded, costs one pass; anything else is sorted once.
func sortedMap[K Key, V any](keys []K, vals []V) (Map[K, V], error) {
	if len(keys) == 0 {
		return Map[K, V]{}, nil
	}
	sorted := true
	for i := 1; i < len(keys) && sorted; i++ {
		sorted = keys[i-1] < keys[i]
	}
	if !sorted {
		order := make([]int, len(keys))
		for i := range order {
			order[i] = i
		}
		slices.SortFunc(order, func(a, b int) int { return cmp.Compare(keys[a], keys[b]) })
		sk, sv := make([]K, len(keys)), make([]V, len(vals))
		for i, j := range order {
			sk[i], sv[i] = keys[j], vals[j]
		}
		keys, vals = sk, sv
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] == keys[i] {
			return Map[K, V]{}, eris.Errorf("immutable map: duplicate key %v", keys[i])
		}
	}
	return Map[K, V]{keys: keys, vals: vals}, nil
}

// keyText returns k as a JSON object key: the string itself, or an integer in decimal.
func keyText[K Key](k K) string {
	v := reflect.ValueOf(k)
	switch v.Kind() { //nolint:exhaustive // Key admits only integers and strings.
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	default:
		return strconv.FormatUint(v.Uint(), 10)
	}
}

// parseKey is the inverse of keyText. An integer must be in canonical decimal, so each key has
// exactly one spelling and "5" and "05" cannot both name it.
func parseKey[K Key](text string) (K, error) {
	var k K
	v := reflect.ValueOf(&k).Elem()
	switch v.Kind() { //nolint:exhaustive // Key admits only integers and strings.
	case reflect.String:
		v.SetString(text)
		return k, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(text, 10, v.Type().Bits())
		if err != nil || strconv.FormatInt(n, 10) != text {
			return k, eris.Errorf("immutable map: key %q is not a %s", text, v.Type())
		}
		v.SetInt(n)
	default:
		n, err := strconv.ParseUint(text, 10, v.Type().Bits())
		if err != nil || strconv.FormatUint(n, 10) != text {
			return k, eris.Errorf("immutable map: key %q is not a %s", text, v.Type())
		}
		v.SetUint(n)
	}
	return k, nil
}
