package immutable

import (
	"bytes"
	"cmp"
	"iter"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unsafe"

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
//	c := npc.Get[NPCMemory]()
//	mem, _ := c.Players.Get(pid)
//	mem.Affinity += 5 // values come out by value
//	c.Players.Set(pid, mem)
//	npc.Set(c)
//
// As with Slice, the storage underneath never changes once a Map holds it: an edit copies only the
// chunk of entries it touches. So copying a Map is free and always safe. d := c.Players and then
// editing d never changes c.Players, and a loop over a Map sees it as it was when the loop started,
// so adding or deleting keys inside the loop is safe; the loop visits exactly the entries that were
// there when it began. An edit reaches the world only through Set.
//
// Entries are kept sorted by key, never hashed for lookup. Iteration and encoding run in key order,
// so the same entries give the same order and the same bytes on every run, machine and build,
// however they were inserted. "First" means smallest key; if insertion order matters, store it in the
// value.
//
// The entries are stored in sorted chunks of about 32, and a Map of more than one chunk keeps them in
// the tree a Slice uses. A lookup is a binary search down that tree and then within one chunk; an edit
// copies one chunk and the path to it, so k edits in an n-entry Map cost about O(k·log n). Where the
// chunks split depends only on the keys, so equal Maps have the same storage layout and
// [reflect.DeepEqual] compares Maps by their entries however they were built.
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
	// Entries sorted by key, never written once held. A Map of one chunk keeps it in one, so an edit
	// copies just that chunk; a Map of more keeps them in chunks, and one is nil.
	one    []entry[K, V]
	chunks Slice[[]entry[K, V]]
	n      int
}

type entry[K Key, V any] struct {
	k K
	v V
}

// -------------------------------------------------------------------------------------------------
// Construction
// -------------------------------------------------------------------------------------------------

// MapFrom returns a Map holding every pair in seq. When a key repeats, the value seq yields last
// wins, so then the result depends on the order. Without repeats it depends only on the pairs, so
// MapFrom(maps.All(m)) is deterministic even though ranging over m is not.
func MapFrom[K Key, V any](seq iter.Seq2[K, V]) Map[K, V] {
	var entries []entry[K, V]
	for k, v := range seq {
		entries = append(entries, entry[K, V]{k, v})
	}
	// Stable, so of the entries sharing a key the one seq yielded last is still last.
	slices.SortStableFunc(entries, func(a, b entry[K, V]) int { return cmp.Compare(a.k, b.k) })
	kept := entries[:0]
	for i, e := range entries {
		if i+1 < len(entries) && entries[i+1].k == e.k {
			continue
		}
		kept = append(kept, e)
	}
	return mapFromSorted(kept)
}

// -------------------------------------------------------------------------------------------------
// Reading
// -------------------------------------------------------------------------------------------------

// Len returns the number of entries.
func (m Map[K, V]) Len() int {
	return m.n
}

// Get returns a copy of the value stored under k, and whether k is present. A missing key yields the
// zero value.
func (m Map[K, V]) Get(k K) (V, bool) {
	if _, chunk, i, ok := m.find(k); ok {
		return chunk[i].v, true
	}
	var zero V
	return zero, false
}

// Has reports whether k is present.
func (m Map[K, V]) Has(k K) bool {
	_, _, _, ok := m.find(k)
	return ok
}

// All returns an iterator over key and value, in ascending key order. Values are yielded by value.
func (m Map[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for c := range m.allChunks() {
			for _, e := range c {
				if !yield(e.k, e.v) {
					return
				}
			}
		}
	}
}

// Keys returns an iterator over the keys, in ascending order.
func (m Map[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range m.All() {
			if !yield(k) {
				return
			}
		}
	}
}

// Values returns an iterator over the values, in ascending key order.
func (m Map[K, V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range m.All() {
			if !yield(v) {
				return
			}
		}
	}
}

// find returns the chunk that holds k, or would, with its index, and k's index in it. The chunk is
// the last one whose first key is at most k, or the first chunk.
func (m Map[K, V]) find(k K) (int, []entry[K, V], int, bool) {
	c, chunk := 0, m.one
	switch {
	case m.one != nil:
	case m.n == 0:
		return 0, nil, 0, false
	default:
		c, chunk = locateChunk(m.chunks, k)
	}
	i, found := searchEntries(chunk, k)
	return c, chunk, i, found
}

// locateChunk returns the index of the last chunk in s whose first key is at most k, or 0, and the
// chunk. s is not empty. It walks the tree once, choosing a kid at each level by its first key.
func locateChunk[K Key, V any](s Slice[[]entry[K, V]], k K) (int, []entry[K, V]) {
	if s.root == nil || s.tail[0][0].k <= k {
		j := lastStartingAtMost(s.tail, k)
		return s.treeLen() + j, s.tail[j]
	}
	node, base := s.root, 0
	for shift := s.shift; shift > 0; shift -= chunkBits {
		lo, hi := 1, len(node.kids)
		for lo < hi {
			mid := int(uint(lo+hi) >> 1)
			if firstKey(node.kids[mid]) <= k {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		base += (lo - 1) << shift
		node = node.kids[lo-1]
	}
	j := lastStartingAtMost(node.items, k)
	return base + j, node.items[j]
}

// firstKey returns the smallest key under node.
func firstKey[K Key, V any](node *vnode[[]entry[K, V]]) K {
	for node.kids != nil {
		node = node.kids[0]
	}
	return node.items[0][0].k
}

// lastStartingAtMost returns the index of the last chunk whose first key is at most k, or 0.
func lastStartingAtMost[K Key, V any](chunks [][]entry[K, V], k K) int {
	lo, hi := 1, len(chunks)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if chunks[mid][0].k <= k {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo - 1
}

// searchEntries returns where k is or would be in the sorted entries, and whether it is there.
func searchEntries[K Key, V any](entries []entry[K, V], k K) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if entries[mid].k < k {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo].k == k
}

// allChunks yields the chunks in key order.
func (m Map[K, V]) allChunks() iter.Seq[[]entry[K, V]] {
	return func(yield func([]entry[K, V]) bool) {
		if m.one != nil {
			yield(m.one)
			return
		}
		for leaf := range m.chunks.chunks() {
			for _, c := range leaf {
				if !yield(c) {
					return
				}
			}
		}
	}
}

func (m Map[K, V]) chunkCount() int {
	if m.one != nil {
		return 1
	}
	return m.chunks.Len()
}

func (m Map[K, V]) chunkAt(c int) []entry[K, V] {
	if m.one != nil {
		return m.one
	}
	return m.chunks.At(c)
}

// spliceChunks replaces chunks [c, end) with out, keeping a single chunk in one.
func (m *Map[K, V]) spliceChunks(c, end int, out [][]entry[K, V]) {
	if m.one == nil {
		m.chunks.splice(c, end, out)
		if m.chunks.Len() == 1 {
			m.one, m.chunks = m.chunks.At(0), Slice[[]entry[K, V]]{}
		}
		return
	}
	switch len(out) {
	case 0:
		m.one = nil
	case 1:
		m.one = out[0]
	default:
		m.one, m.chunks = nil, fromItems(slices.Clone(out))
	}
}

// -------------------------------------------------------------------------------------------------
// Editing
// -------------------------------------------------------------------------------------------------

// Set stores v under k, replacing any value already there.
func (m *Map[K, V]) Set(k K, v V) {
	c, old, i, found := m.find(k)
	if m.n == 0 {
		*m = mapFromSorted([]entry[K, V]{{k, v}})
		return
	}
	if found {
		edited := slices.Clone(old)
		edited[i].v = v
		if m.one != nil {
			m.one = edited
		} else {
			m.chunks.Set(c, edited)
		}
		return
	}
	edited := make([]entry[K, V], 0, len(old)+1)
	edited = append(append(append(edited, old[:i]...), entry[K, V]{k, v}), old[i:]...)
	m.rechunk(c, edited)
	m.n++
}

// Delete removes k. It does nothing when k is absent, like delete.
func (m *Map[K, V]) Delete(k K) {
	c, old, i, found := m.find(k)
	if !found {
		return
	}
	if m.n == 1 {
		*m = Map[K, V]{}
		return
	}
	edited := make([]entry[K, V], 0, len(old)-1)
	edited = append(append(edited, old[:i]...), old[i+1:]...)
	m.rechunk(c, edited)
	m.n--
}

// DeleteFunc removes every entry del reports true for, like [maps.DeleteFunc]. del runs once per
// entry, in key order, and the whole pass costs O(n) however many entries go.
func (m *Map[K, V]) DeleteFunc(del func(K, V) bool) {
	var kept []entry[K, V]
	seen, deleted := 0, false
	for c := range m.allChunks() {
		for _, e := range c {
			switch {
			case del(e.k, e.v):
				if !deleted {
					deleted = true
					kept = appendFirstEntries(make([]entry[K, V], 0, m.n-1), *m, seen)
				}
			case deleted:
				kept = append(kept, e)
			}
			seen++
		}
	}
	if deleted {
		*m = mapFromSorted(kept)
	}
}

// appendFirstEntries appends the first k entries of m to dst.
func appendFirstEntries[K Key, V any](dst []entry[K, V], m Map[K, V], k int) []entry[K, V] {
	for c := range m.allChunks() {
		if k <= len(c) {
			return append(dst, c[:k]...)
		}
		dst = append(dst, c...)
		k -= len(c)
	}
	return dst
}

// Clear removes every entry, like Go's clear on a map and [sync.Map.Clear].
func (m *Map[K, V]) Clear() {
	*m = Map[K, V]{}
}

// -------------------------------------------------------------------------------------------------
// Chunking
// -------------------------------------------------------------------------------------------------
// Chunk boundaries are content-defined, as in a prolly tree: once a chunk holds mapChunkMin entries,
// it ends after the next entry whose key hashes to a boundary (one key in mapChunkSpread), or when it
// reaches mapChunkMax entries. The layout is then a function of the sorted keys alone. An edit only
// changes chunks from the edited one up to where a cut lands at the end of an old chunk, after which
// the old and new layouts agree, so it rebuilds one or two chunks however large the Map is. The
// minimum keeps every Map of up to mapChunkMin entries in one chunk, where an edit allocates once.

const (
	mapChunkMin    = 16
	mapChunkSpread = 16 // a power of two; chunks average mapChunkMin+mapChunkSpread entries
	mapChunkMax    = 128
)

// endsChunk reports whether a chunk of size entries, the last with key k, ends there.
func endsChunk[K Key](size int, k K) bool {
	return size == mapChunkMax || size >= mapChunkMin && keyHash(k)&(mapChunkSpread-1) == 0
}

// rechunk replaces chunk c with the chunks edited splits into, continuing into the chunks after it
// until a boundary lands where an old chunk ended. edited is a new array that rechunk takes ownership
// of; the new chunks are windows onto it, capped so that a later append cannot reach a neighbour.
func (m *Map[K, V]) rechunk(c int, edited []entry[K, V]) {
	var buf [4][]entry[K, V]
	out := buf[:0]
	end, start, scan := c+1, 0, 0
	for {
		for ; scan < len(edited); scan++ {
			if endsChunk(scan+1-start, edited[scan].k) {
				out = append(out, edited[start:scan+1:scan+1])
				start = scan + 1
			}
		}
		if start == len(edited) || end == m.chunkCount() {
			break
		}
		// Growing edited may move it, which leaves the windows already cut on the old array. That is
		// fine: nothing writes to the old array again.
		edited = append(edited, m.chunkAt(end)...)
		end++
	}
	if start < len(edited) {
		out = append(out, edited[start:len(edited):len(edited)])
	}
	m.spliceChunks(c, end, out)
}

// mapFromSorted returns the Map holding entries, which must be sorted by key without repeats. It
// takes ownership of the array: the chunks are windows onto it.
func mapFromSorted[K Key, V any](entries []entry[K, V]) Map[K, V] {
	if len(entries) == 0 {
		return Map[K, V]{}
	}
	var chunks [][]entry[K, V]
	start := 0
	for i, e := range entries {
		if endsChunk(i+1-start, e.k) {
			chunks = append(chunks, entries[start:i+1:i+1])
			start = i + 1
		}
	}
	if start < len(entries) {
		chunks = append(chunks, entries[start:len(entries):len(entries)])
	}
	if len(chunks) == 1 {
		return Map[K, V]{one: chunks[0], n: len(entries)}
	}
	return Map[K, V]{chunks: fromItems(chunks), n: len(entries)}
}

// keyHash hashes k the same way on every machine and build: FNV-1a over a string's bytes, or the
// integer's value as an int64 or uint64, then the splitmix64 finalizer.
func keyHash[K Key](k K) uint64 {
	p := unsafe.Pointer(&k)
	var x uint64
	var signed int64
	switch reflect.TypeFor[K]().Kind() { //nolint:exhaustive // Key admits only integers and strings.
	case reflect.String:
		x = 14695981039346656037
		for _, b := range []byte(*(*string)(p)) {
			x ^= uint64(b)
			x *= 1099511628211
		}
	case reflect.Int:
		signed = int64(*(*int)(p))
		x = uint64(signed) //nolint:gosec // The bits are hashed, never used as a number.
	case reflect.Int8:
		signed = int64(*(*int8)(p))
		x = uint64(signed) //nolint:gosec // As above.
	case reflect.Int16:
		signed = int64(*(*int16)(p))
		x = uint64(signed) //nolint:gosec // As above.
	case reflect.Int32:
		signed = int64(*(*int32)(p))
		x = uint64(signed) //nolint:gosec // As above.
	case reflect.Int64:
		x = uint64(*(*int64)(p)) //nolint:gosec // As above.
	case reflect.Uint:
		x = uint64(*(*uint)(p))
	case reflect.Uint8:
		x = uint64(*(*uint8)(p))
	case reflect.Uint16:
		x = uint64(*(*uint16)(p))
	case reflect.Uint32:
		x = uint64(*(*uint32)(p))
	default:
		x = *(*uint64)(p)
	}
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// -------------------------------------------------------------------------------------------------
// Comparing
// -------------------------------------------------------------------------------------------------

// EqualFunc reports whether both maps hold the same keys and eq holds for the values under each.
func (m Map[K, V]) EqualFunc(other Map[K, V], eq func(a, b V) bool) bool {
	if m.n != other.n || m.chunkCount() != other.chunkCount() {
		return false
	}
	// Chunk boundaries depend only on the keys, so equal key sets are chunked alike.
	i := 0
	for a := range m.allChunks() {
		b := other.chunkAt(i)
		i++
		if len(a) != len(b) {
			return false
		}
		for j := range a {
			if a[j].k != b[j].k || !eq(a[j].v, b[j].v) {
				return false
			}
		}
	}
	return true
}

// MapEqual reports whether a and b hold the same entries. It is a function rather than a method
// because a method cannot narrow V to comparable; use EqualFunc for other value types.
func MapEqual[K Key, V comparable](a, b Map[K, V]) bool {
	return a.EqualFunc(b, func(x, y V) bool { return x == y })
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
	i := 0
	for k, v := range m.All() {
		if i > 0 {
			buf.WriteByte(',')
		}
		i++
		key, err := json.Marshal(keyText(k))
		if err != nil {
			return nil, eris.Wrap(err, "failed to encode immutable map key")
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := json.Marshal(v)
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
	if err := enc.EncodeMapLen(m.n); err != nil {
		return eris.Wrap(err, "failed to encode immutable map length")
	}
	for k, v := range m.All() {
		if err := enc.Encode(k); err != nil {
			return eris.Wrap(err, "failed to encode immutable map key")
		}
		if err := enc.Encode(v); err != nil {
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
func (m Map[K, V]) IsZero() bool { return m.n == 0 }

// MapTypes reports whether t is an instantiation of Map and, if so, returns its key and value types.
// Like SliceElem, it lets reflection-based tooling recognise a Map without depending on its layout.
func MapTypes(t reflect.Type) (reflect.Type, reflect.Type, bool) {
	if t == nil || t.Kind() != reflect.Struct || !strings.HasPrefix(t.Name(), "Map[") {
		return nil, nil, false
	}
	if t.PkgPath() != reflect.TypeFor[Map[int, struct{}]]().PkgPath() {
		return nil, nil, false
	}
	one, ok := t.FieldByName("one")
	if !ok || one.Type.Kind() != reflect.Slice || one.Type.Elem().Kind() != reflect.Struct ||
		one.Type.Elem().NumField() != 2 {
		return nil, nil, false
	}
	e := one.Type.Elem()
	return e.Field(0).Type, e.Field(1).Type, true
}

// sortedMap builds a Map from decoded entries in whatever order they arrived. Input that is already
// sorted, which is everything this package encoded, costs one pass; anything else is sorted once.
func sortedMap[K Key, V any](keys []K, vals []V) (Map[K, V], error) {
	entries := make([]entry[K, V], len(keys))
	sorted := true
	for i, k := range keys {
		entries[i] = entry[K, V]{k, vals[i]}
		sorted = sorted && (i == 0 || keys[i-1] < k)
	}
	if !sorted {
		slices.SortFunc(entries, func(a, b entry[K, V]) int { return cmp.Compare(a.k, b.k) })
		// Only sorting can bring duplicates together; input that passed the strict < check has none.
		for i := 1; i < len(entries); i++ {
			if entries[i-1].k == entries[i].k {
				return Map[K, V]{}, eris.Errorf("immutable map: duplicate key %v", entries[i].k)
			}
		}
	}
	return mapFromSorted(entries), nil
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
