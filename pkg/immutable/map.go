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
// and structs because they have no < at all. That makes Key [cmp.Ordered] without floats and
// uintptr. Rust's BTreeMap leaves out floats for the same reason: f64 is not Ord.
type Key interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~string
}

// Map is a sorted map with private storage, for component fields that look values up by key.
//
// Edit it in place, the way you would a Go map, and store it with Set:
//
//	c := npc.Get[NPCMemory]() // c.Players is a copy; the world is unchanged
//	mem, _ := c.Players.Get(pid)
//	mem.Affinity += 5         // edits the copy Get returned
//	c.Players.Set(pid, mem)
//	npc.Set(c)                // the world stores a copy of c.Players
//
// Cardinal copies every Map a component holds on Get and Set, and in events and commands, exactly as
// it copies a Slice, so an edit never reaches the world without Set. Inside one system, assigning a
// Map shares its storage like a Go slice: a value changed through one copy can show in the other,
// though a key added or removed may not. Clone makes a copy that shares nothing. Deleting inside a
// loop over the Map is safe, as with a Go map; adding keys there is not (see Set).
//
// Entries are kept sorted by key, never hashed. Iteration and encoding run in key order, so the same
// entries give the same order and the same bytes on every run, machine and build, however they were
// inserted. "First" means smallest key; if insertion order matters, store it in the value.
//
// The storage is two sorted arrays, as in C++'s std::flat_map: a lookup is a binary search, and a
// copy is two plain array copies, which matters because Cardinal copies on every Get and Set. Adding
// or removing a key shifts the entries after it, so k new keys in an n-entry Map cost O(k·n). To
// build a large Map at once, use MapFrom, which sorts once.
//
// The zero value is an empty Map, and every empty Map is the zero value.
//
// Don't key a Map by an entity ID unless the system that destroys those entities also removes their
// keys. Entity IDs are reused as soon as an entity is destroyed, so a stale key silently refers to
// whichever entity takes the slot next. Key long-lived data by an ID the game owns: a player ID, a
// database ID, or a counter the game mints that never repeats.
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

// MapFrom returns a Map holding every pair in seq. When a key repeats, the value seq yields last
// wins, so then the result depends on the order. Without repeats it depends only on the pairs, so
// MapFrom(maps.All(m)) is deterministic even though ranging over m is not.
func MapFrom[K Key, V any](seq iter.Seq2[K, V]) Map[K, V] {
	type entry struct {
		k K
		v V
	}
	var entries []entry
	for k, v := range seq {
		entries = append(entries, entry{k, v})
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
	return wrapMap(keys, vals)
}

// Clone returns a copy of m that shares no storage with it, including the storage of Slices and Maps
// nested in its values.
func (m Map[K, V]) Clone() Map[K, V] {
	m.cloneDeep()
	return m
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
// Editing
// -------------------------------------------------------------------------------------------------

// Set stores v under k, replacing any value already there. Replacing costs a binary search; a new key
// also shifts the entries after it, in place when the arrays have room. So, unlike a Go map, don't
// add keys inside a loop over m: the loop can skip or repeat entries. Collect the keys to add first,
// or loop over [slices.Collect](m.Keys()).
func (m *Map[K, V]) Set(k K, v V) {
	i, ok := slices.BinarySearch(m.keys, k)
	if ok {
		m.vals[i] = v
		return
	}
	m.keys = slices.Insert(m.keys, i, k)
	m.vals = slices.Insert(m.vals, i, v)
}

// Delete removes k. It does nothing when k is absent, like delete.
//
// It is safe inside a loop over m, as it is for a Go map: the loop still visits every other entry
// once. Removing a key moves the rest into new arrays rather than shifting them in place, so a loop
// over m, or another copy of m, keeps reading the old arrays, which stay valid.
func (m *Map[K, V]) Delete(k K) {
	i, ok := slices.BinarySearch(m.keys, k)
	if !ok {
		return
	}
	m.keys = emptyToNil(slices.Concat(m.keys[:i], m.keys[i+1:]))
	m.vals = emptyToNil(slices.Concat(m.vals[:i], m.vals[i+1:]))
}

// DeleteFunc removes every entry del reports true for, like [maps.DeleteFunc]. del runs once per
// entry, in key order. The whole pass costs O(n) and one new pair of arrays however many entries go,
// so prefer it to Delete in a loop when removing many. Like Delete, it leaves the old arrays intact.
func (m *Map[K, V]) DeleteFunc(del func(K, V) bool) {
	var keys []K
	var vals []V
	for i, k := range m.keys {
		switch {
		case !del(k, m.vals[i]):
			if keys != nil {
				keys, vals = append(keys, k), append(vals, m.vals[i])
			}
		case keys == nil:
			// The first deletion: copy the entries kept so far into new arrays sized for the rest.
			keys = append(make([]K, 0, len(m.keys)-1), m.keys[:i]...)
			vals = append(make([]V, 0, len(m.vals)-1), m.vals[:i]...)
		}
	}
	if keys == nil {
		return // nothing deleted
	}
	m.keys, m.vals = emptyToNil(keys), emptyToNil(vals)
}

// Clear removes every entry, like Go's clear on a map and [sync.Map.Clear].
func (m *Map[K, V]) Clear() {
	m.keys, m.vals = nil, nil
}

// wrapMap returns a Map holding keys and vals, or the zero value when they are empty.
func wrapMap[K Key, V any](keys []K, vals []V) Map[K, V] {
	if len(keys) == 0 {
		return Map[K, V]{}
	}
	return Map[K, V]{keys: keys, vals: vals}
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
// encode to equal bytes. Both decoders accept entries in any order and sort them once. A repeated key
// is an error: there is no right value to keep. Changing a key's type between redeploys is not safe:
// MessagePack converts an integer key as Go converts integers, so -1 stored as int32 loads as
// 4294967295 into uint32, with no error. As with Slice, the JSON methods exist because otherwise the
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
		// DecodeValue, not Decode: Decode handles *float32 itself and skips decoders registered with
		// msgpack.Register, such as cardinal's, which accepts a 64-bit float into a float32.
		if err := dec.DecodeValue(reflect.ValueOf(&v).Elem()); err != nil {
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
		// Only sorting can bring duplicates together; input that passed the strict < check has none.
		for i := 1; i < len(keys); i++ {
			if keys[i-1] == keys[i] {
				return Map[K, V]{}, eris.Errorf("immutable map: duplicate key %v", keys[i])
			}
		}
	}
	return wrapMap(keys, vals), nil
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
