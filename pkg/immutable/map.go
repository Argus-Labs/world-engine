package immutable

import (
	"bytes"
	"cmp"
	"hash/maphash"
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
// The entries are stored in sorted chunks of about 32, and a Map of more than one chunk indexes them
// with a tree keyed by each chunk's first key. A lookup is a binary search down that tree and then
// within one chunk; an edit copies one chunk and the path to it, so k edits in an n-entry Map cost
// about O(k·log n). Where the chunks and the tree's nodes split depends only on the keys and a seed
// drawn when the program starts, so equal Maps have the same storage layout and [reflect.DeepEqual]
// compares Maps by their entries however they were built.
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
	// copies just that chunk; a Map of more indexes them under root, and one is nil.
	one  []entry[K, V]
	root *mnode[K, V]
	n    int
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
	if chunk, i, ok := m.find(k); ok {
		return chunk[i].v, true
	}
	var zero V
	return zero, false
}

// Has reports whether k is present.
func (m Map[K, V]) Has(k K) bool {
	_, _, ok := m.find(k)
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

// MaxKey returns the largest key, and false when the Map is empty. It follows the last child down the
// index, O(log n), where ranging over Keys to the end costs O(n). The smallest key needs no method:
// it is the first one Keys yields.
func (m Map[K, V]) MaxKey() (K, bool) {
	chunk := m.one
	if chunk == nil {
		if m.root == nil {
			var zero K
			return zero, false
		}
		node := m.root
		for node.kids != nil {
			node = node.kids[len(node.kids)-1]
		}
		chunk = node.chunks[len(node.chunks)-1]
	}
	return chunk[len(chunk)-1].k, true
}

// Range returns an iterator over key and value for the keys in [lo, hi), in ascending key order. It
// goes straight to lo, so reading k entries costs O(log n + k), where ranging over All and skipping
// to lo costs O(n). It yields nothing when lo >= hi.
func (m Map[K, V]) Range(lo, hi K) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		if m.n == 0 || lo >= hi {
			return
		}
		var p mpath[K, V]
		chunk := m.one
		if chunk == nil {
			chunk = m.locate(lo, &p)
		}
		i, _ := searchEntries(chunk, lo)
		for {
			for _, e := range chunk[i:] {
				if e.k >= hi || !yield(e.k, e.v) {
					return
				}
			}
			if m.one != nil || !advance(&p.f, 0, p.h) {
				return
			}
			chunk, i = p.f[0].node.chunks[p.f[0].i], 0
		}
	}
}

// find returns the chunk that holds k, or would, and k's index in it.
func (m Map[K, V]) find(k K) ([]entry[K, V], int, bool) {
	chunk := m.one
	if chunk == nil {
		if m.root == nil {
			return nil, 0, false
		}
		node := m.root
		for node.kids != nil {
			node = node.kids[lastAtMost(node.keys, k)]
		}
		chunk = node.chunks[lastAtMost(node.keys, k)]
	}
	i, found := searchEntries(chunk, k)
	return chunk, i, found
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
		switch {
		case m.one != nil:
			yield(m.one)
		case m.root != nil:
			walkChunks(m.root, yield)
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Editing
// -------------------------------------------------------------------------------------------------

// Set stores v under k, replacing any value already there.
func (m *Map[K, V]) Set(k K, v V) {
	if m.n == 0 {
		*m = mapFromSorted([]entry[K, V]{{k, v}})
		return
	}
	var p mpath[K, V]
	old := m.one
	if old == nil {
		old = m.locate(k, &p)
	}
	i, found := searchEntries(old, k)
	if found {
		edited := slices.Clone(old)
		edited[i].v = v
		if m.one != nil {
			m.one = edited
		} else {
			m.replaceAbove(&p, 0, mchild[K, V]{chunk: edited})
		}
		return
	}
	edited := make([]entry[K, V], 0, len(old)+1)
	edited = append(append(append(edited, old[:i]...), entry[K, V]{k, v}), old[i:]...)
	m.rechunk(&p, edited)
	m.n++
}

// Delete removes k. It does nothing when k is absent, like delete.
func (m *Map[K, V]) Delete(k K) {
	if m.n == 0 {
		return
	}
	var p mpath[K, V]
	old := m.one
	if old == nil {
		old = m.locate(k, &p)
	}
	i, found := searchEntries(old, k)
	if !found {
		return
	}
	if m.n == 1 {
		*m = Map[K, V]{}
		return
	}
	edited := make([]entry[K, V], 0, len(old)-1)
	edited = append(append(edited, old[:i]...), old[i+1:]...)
	m.rechunk(&p, edited)
	m.n--
}

// rechunk replaces the edited chunk, the one at p or the Map's only one, with the chunks edited
// splits into. edited is a new array that rechunk takes ownership of; the new chunks are windows onto
// it, capped so that a later append cannot reach a neighbour.
func (m *Map[K, V]) rechunk(p *mpath[K, V], edited []entry[K, V]) {
	if m.one == nil {
		m.rechunkTree(p, edited)
		return
	}
	var buf [4]mchild[K, V]
	m.setTop(cutEntries(buf[:0], edited), 0)
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

// UpdateFunc replaces each value v under key k with f(k, v), in key order, like ranging over a Go map
// and storing each result. No key changes, so the Map keeps its shape: it rebuilds once, O(n) with one
// allocation for the entries and one per index node, where calling Set for every key copies a chunk
// and its path each time. f sees the Map as it was before the call, and a panic in f leaves the Map
// unchanged.
func (m *Map[K, V]) UpdateFunc(f func(k K, v V) V) {
	if m.n == 0 {
		return
	}
	all := make([]entry[K, V], 0, m.n)
	if m.one != nil {
		m.one = updateChunk(&all, m.one, f)
		return
	}
	m.root = updateNode(&all, m.root, f)
}

// updateChunk appends c's entries to *all with their values replaced by f, and returns them as a
// window onto *all, capped so a later append cannot reach a neighbour. *all never grows past the
// capacity it was made with, so earlier windows stay valid.
func updateChunk[K Key, V any](all *[]entry[K, V], c []entry[K, V], f func(K, V) V) []entry[K, V] {
	start := len(*all)
	for _, e := range c {
		*all = append(*all, entry[K, V]{e.k, f(e.k, e.v)})
	}
	return (*all)[start:len(*all):len(*all)]
}

// updateNode returns a copy of node with the same keys and every value below it replaced by f.
func updateNode[K Key, V any](all *[]entry[K, V], node *mnode[K, V], f func(K, V) V) *mnode[K, V] {
	b := allocNode[K, V](len(node.keys), node.kids == nil)
	copy(b.keys, node.keys)
	if node.kids == nil {
		for i, c := range node.chunks {
			b.chunks[i] = updateChunk(all, c, f)
		}
	} else {
		for i, kid := range node.kids {
			b.kids[i] = updateNode(all, kid, f)
		}
	}
	return b
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

// mapFromSorted returns the Map holding entries, which must be sorted by key without repeats. It
// takes ownership of the array: the chunks are windows onto it.
func mapFromSorted[K Key, V any](entries []entry[K, V]) Map[K, V] {
	if len(entries) == 0 {
		return Map[K, V]{}
	}
	var buf [4]mchild[K, V]
	m := Map[K, V]{n: len(entries)}
	m.setTop(cutEntries(buf[:0], entries), 0)
	return m
}

// hashSeed is drawn at random when the program starts. The source is public, so with a fixed hash
// anyone could work out offline which keys never end a chunk, and a Map built from such keys, say
// player names, would cut every chunk at mapChunkMax by position: then an edit near the front shifts
// every later boundary and re-cuts the whole Map, O(n) instead of O(log n). Without the seed those
// keys can't be found. The seed only moves chunk boundaries, which nothing outside the process sees:
// order, encoding and snapshots follow the keys, and every Map in one process shares the seed, so
// equal Maps still have equal layouts.
var hashSeed = maphash.MakeSeed() //nolint:gochecknoglobals // one per process, so equal Maps lay out alike

// keyHash is the seeded hash that places chunk and index boundaries.
func keyHash[K Key](k K) uint64 { return maphash.Comparable(hashSeed, k) }

// -------------------------------------------------------------------------------------------------
// Comparing
// -------------------------------------------------------------------------------------------------

// EqualFunc reports whether both maps hold the same keys and eq holds for the values under each.
// Storage that both share counts as equal without calling eq, as in [reflect.DeepEqual], so comparing
// a value with an unchanged copy is O(1) and with an edited copy costs about one chunk. A NaN held in
// shared storage therefore compares equal.
func (m Map[K, V]) EqualFunc(other Map[K, V], eq func(a, b V) bool) bool {
	if m.n != other.n || (m.root == nil) != (other.root == nil) {
		return false
	}
	// Chunk and node boundaries depend only on the keys, so equal key sets are laid out alike.
	if m.root == nil {
		return chunksEqual(m.one, other.one, eq)
	}
	return nodesEqual(m.root, other.root, eq)
}

// MapEqual reports whether a and b hold the same entries. It is a function rather than a method
// because a method cannot narrow V to comparable; use EqualFunc for other value types. Shared storage
// counts as equal, as in EqualFunc.
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
	var entries []entry[K, V]
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
		entries = append(entries, entry[K, V]{k: k})
		if err := dec.Decode(&entries[len(entries)-1].v); err != nil {
			return eris.Wrapf(err, "failed to decode immutable map value for key %q", text)
		}
	}
	if _, err := dec.Token(); err != nil {
		return eris.Wrap(err, "failed to decode immutable map end")
	}
	decoded, err := sortedMap(entries)
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

// DecodeMsgpack replaces the Map only after every entry has decoded successfully. Each entry decodes
// in place in the array, so decoding allocates per array growth, not per entry. The claimed length
// only sizes the first allocation, up to a cap, so a forged length cannot force a large one. nil and
// an empty map yield the zero value.
func (m *Map[K, V]) DecodeMsgpack(dec *msgpack.Decoder) error {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return eris.Wrap(err, "failed to decode immutable map length")
	}
	entries := make([]entry[K, V], 0, min(max(n, 0), decodePrealloc))
	for range n {
		entries = append(entries, entry[K, V]{})
		e := &entries[len(entries)-1]
		if err := dec.Decode(&e.k); err != nil {
			return eris.Wrap(err, "failed to decode immutable map key")
		}
		// DecodeValue, not Decode: Decode handles *float32 itself and skips decoders registered with
		// msgpack.Register, such as cardinal's, which accepts a 64-bit float into a float32.
		if err := dec.DecodeValue(reflect.ValueOf(&e.v).Elem()); err != nil {
			return eris.Wrap(err, "failed to decode immutable map value")
		}
	}
	decoded, err := sortedMap(entries)
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

// sortedMap builds a Map from decoded entries in whatever order they arrived, taking ownership of the
// array. Input that is already sorted, which is everything this package encoded, costs one pass;
// anything else is sorted once.
func sortedMap[K Key, V any](entries []entry[K, V]) (Map[K, V], error) {
	sorted := true
	for i := 1; i < len(entries) && sorted; i++ {
		sorted = entries[i-1].k < entries[i].k
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
