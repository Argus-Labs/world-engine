package immutable_test

import (
	"bytes"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

type playerID uint64

type memory struct {
	Affinity int
	Seen     immutable.Slice[string]
}

// entries copies a Map out to parallel key and value slices, in the Map's order.
func entries[K immutable.Key, V any](m immutable.Map[K, V]) ([]K, []V) {
	return slices.Collect(m.Keys()), slices.Collect(m.Values())
}

func mapOf[K immutable.Key, V any](kv map[K]V) immutable.Map[K, V] {
	return immutable.MapFrom(maps.All(kv))
}

func TestMap_ZeroValueIsEmpty(t *testing.T) {
	t.Parallel()

	var m immutable.Map[string, int]
	assert.Zero(t, m.Len())
	assert.False(t, m.Has("a"))
	v, ok := m.Get("a")
	assert.False(t, ok)
	assert.Zero(t, v)
	for range m.All() {
		t.Fatal("the zero Map has no entries")
	}
}

func TestMap_ReadsAndKeyOrder(t *testing.T) {
	t.Parallel()

	var m immutable.Map[int, string]
	m.Set(3, "c")
	m.Set(1, "a")
	m.Set(2, "b")
	keys, vals := entries(m)
	assert.Equal(t, []int{1, 2, 3}, keys)
	assert.Equal(t, []string{"a", "b", "c"}, vals)
	assert.Equal(t, 3, m.Len())
	assert.True(t, m.Has(2))
	assert.False(t, m.Has(4))
	v, ok := m.Get(2)
	assert.True(t, ok)
	assert.Equal(t, "b", v)

	var seen []int
	for k := range m.All() {
		seen = append(seen, k)
		if k == 2 {
			break
		}
	}
	assert.Equal(t, []int{1, 2}, seen, "All stops when the loop does")
}

func TestMap_Edits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(m *immutable.Map[string, int])
		want map[string]int
	}{
		{"Set existing", func(m *immutable.Map[string, int]) { m.Set("b", 9) }, map[string]int{"a": 1, "b": 9, "c": 3}},
		{"Set new first", func(m *immutable.Map[string, int]) { m.Set("0", 9) },
			map[string]int{"0": 9, "a": 1, "b": 2, "c": 3}},
		{"Set new middle", func(m *immutable.Map[string, int]) { m.Set("bb", 9) },
			map[string]int{"a": 1, "b": 2, "bb": 9, "c": 3}},
		{"Set new last", func(m *immutable.Map[string, int]) { m.Set("d", 9) },
			map[string]int{"a": 1, "b": 2, "c": 3, "d": 9}},
		{"Delete", func(m *immutable.Map[string, int]) { m.Delete("b") }, map[string]int{"a": 1, "c": 3}},
		{"Delete absent", func(m *immutable.Map[string, int]) { m.Delete("zz") },
			map[string]int{"a": 1, "b": 2, "c": 3}},
		{"DeleteFunc", func(m *immutable.Map[string, int]) {
			m.DeleteFunc(func(_ string, v int) bool { return v%2 == 1 })
		}, map[string]int{"b": 2}},
		{"DeleteFunc none", func(m *immutable.Map[string, int]) {
			m.DeleteFunc(func(string, int) bool { return false })
		}, map[string]int{"a": 1, "b": 2, "c": 3}},
		{"Clear", func(m *immutable.Map[string, int]) { m.Clear() }, map[string]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := mapOf(map[string]int{"a": 1, "b": 2, "c": 3})
			tt.edit(&m)
			require.Equal(t, tt.want, maps.Collect(m.All()))
			keys, _ := entries(m)
			require.True(t, slices.IsSorted(keys), "keys stay sorted: %v", keys)
		})
	}
}

func TestMap_DeleteFuncVisitsInKeyOrder(t *testing.T) {
	t.Parallel()

	m := mapOf(map[string]int{"c": 3, "a": 1, "d": 4, "b": 2})
	var seen []string
	m.DeleteFunc(func(k string, _ int) bool {
		seen = append(seen, k)
		return k == "b"
	})
	require.Equal(t, []string{"a", "b", "c", "d"}, seen, "del runs once per entry, in key order")
}

func TestMap_EmptyIsZeroValue(t *testing.T) {
	t.Parallel()

	one := func() immutable.Map[string, int] {
		var m immutable.Map[string, int]
		m.Set("a", 1)
		return m
	}
	deleted, filtered, cleared := one(), one(), one()
	deleted.Delete("a")
	filtered.DeleteFunc(func(string, int) bool { return true })
	cleared.Clear()

	var fromMsgpack, fromJSON immutable.Map[string, int]
	require.NoError(t, msgpack.Unmarshal([]byte{0x80}, &fromMsgpack))
	require.NoError(t, fromJSON.UnmarshalJSON([]byte(`{}`)))

	for name, m := range map[string]immutable.Map[string, int]{
		"Delete last":   deleted,
		"DeleteFunc":    filtered,
		"Clear":         cleared,
		"Clone empty":   immutable.Map[string, int]{}.Clone(),
		"MapFrom empty": immutable.MapFrom(maps.All(map[string]int{})),
		"msgpack {}":    fromMsgpack,
		"JSON {}":       fromJSON,
	} {
		assert.True(t, reflect.DeepEqual(immutable.Map[string, int]{}, m), name)
	}
}

func TestMap_AssignmentSharesAndCloneDoesNot(t *testing.T) {
	t.Parallel()

	m := mapOf(map[string]int{"a": 1, "b": 2})
	shared := m
	shared.Set("a", 9)
	v, _ := m.Get("a")
	require.Equal(t, 9, v, "an assignment shares the storage, as with a Go map")

	clone := m.Clone()
	clone.Set("a", 1)
	clone.Set("c", 3)
	clone.Delete("b")
	keys, vals := entries(m)
	require.Equal(t, []string{"a", "b"}, keys, "edits to a clone do not reach the original")
	require.Equal(t, []int{9, 2}, vals)

	// Adding or deleting keys through one copy leaves the other a valid Map: sorted, every key found,
	// and encodable to a snapshot that restores.
	for name, edit := range map[string]func(*immutable.Map[int, int]){
		"Delete first":   func(m *immutable.Map[int, int]) { m.Delete(0) },
		"Delete middle":  func(m *immutable.Map[int, int]) { m.Delete(2) },
		"DeleteFunc":     func(m *immutable.Map[int, int]) { m.DeleteFunc(func(k, _ int) bool { return k%2 == 1 }) },
		"Set new first":  func(m *immutable.Map[int, int]) { m.Set(-1, -1) },
		"Set new middle": func(m *immutable.Map[int, int]) { m.Set(5, 5) },
		"Clear":          func(m *immutable.Map[int, int]) { m.Clear() },
	} {
		var orig immutable.Map[int, int]
		for _, k := range []int{0, 2, 4, 6, 8} {
			orig.Set(k, k)
		}
		other := orig
		edit(&other)
		requireValidMap(t, orig, name+", the other copy")
		requireValidMap(t, other, name+", the edited copy")
	}
}

// TestMap_RangeAndDelete is how game code expires entries in a Go map: delete the current key while
// ranging. Every entry must be visited once, and only the expired ones removed.
func TestMap_RangeAndDelete(t *testing.T) {
	t.Parallel()

	var buffs immutable.Map[int, int] // buff ID to the tick it expires at
	for id, at := range map[int]int{0: 1000, 3: 5, 4: 6, 7: 1000, 9: 2} {
		buffs.Set(id, at)
	}
	var visited []int
	for id, at := range buffs.All() {
		visited = append(visited, id)
		if at <= 10 {
			buffs.Delete(id)
		}
	}
	require.Equal(t, []int{0, 3, 4, 7, 9}, visited, "every buff is visited once, with no extra entry")
	keys, _ := entries(buffs)
	require.Equal(t, []int{0, 7}, keys, "only the expired buffs are removed")
}

func TestMap_CloneCopiesNestedCollections(t *testing.T) {
	t.Parallel()

	var m immutable.Map[int, memory]
	m.Set(1, memory{Seen: immutable.SliceOf("x")})
	clone := m.Clone()
	mem, _ := clone.Get(1)
	mem.Seen.Set(0, "edited")

	orig, _ := m.Get(1)
	require.Equal(t, "x", orig.Seen.At(0), "a Slice inside a value is copied too")
}

func TestMapFrom_LastValueWins(t *testing.T) {
	t.Parallel()

	pairs := func(yield func(string, int) bool) {
		for _, p := range []struct {
			k string
			v int
		}{{"b", 1}, {"a", 1}, {"b", 2}, {"a", 2}, {"b", 3}} {
			if !yield(p.k, p.v) {
				return
			}
		}
	}
	keys, vals := entries(immutable.MapFrom(pairs))
	assert.Equal(t, []string{"a", "b"}, keys)
	assert.Equal(t, []int{2, 3}, vals)
}

func TestMap_EqualFunc(t *testing.T) {
	t.Parallel()

	one := func() immutable.Map[int, memory] {
		var m immutable.Map[int, memory]
		m.Set(1, memory{Seen: immutable.SliceOf("x")})
		return m
	}
	eq := func(x, y memory) bool { return x.Affinity == y.Affinity && immutable.Equal(x.Seen, y.Seen) }
	assert.True(t, one().EqualFunc(one(), eq))
	extra, changed := one(), one()
	extra.Set(2, memory{})
	changed.Set(1, memory{Affinity: 1})
	assert.False(t, one().EqualFunc(extra, eq))
	assert.False(t, one().EqualFunc(changed, eq))
}

// TestMap_ModelMatchesGoMap runs random edits against a Go map read back in sorted key order. Some
// delete inside a loop over the Map, as Go allows for its maps. Earlier copies of the Map are kept,
// and after every edit each must still be a valid Map, even though it shares storage.
func TestMap_ModelMatchesGoMap(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	var m immutable.Map[int, int]
	model := map[int]int{}
	var copies []immutable.Map[int, int]
	for step := range 3000 {
		k, v := r.IntN(40), r.IntN(1000)
		switch op := r.IntN(12); {
		case op < 4:
			m.Set(k, v)
			model[k] = v
		case op < 6:
			got, _ := m.Get(k)
			m.Set(k, got+v)
			model[k] += v
		case op < 8:
			m.Delete(k)
			delete(model, k)
		case op == 8:
			mod := 3 + r.IntN(8)
			del := func(k, _ int) bool { return k%mod == 0 }
			m.DeleteFunc(del)
			maps.DeleteFunc(model, del)
		case op == 9:
			mod := 2 + r.IntN(4)
			del := func(k, v int) bool { return k%mod == 0 || v < 300 }
			want := slices.Collect(m.Keys())
			var visited []int
			for k, v := range m.All() {
				visited = append(visited, k)
				if del(k, v) {
					m.Delete(k)
				}
			}
			maps.DeleteFunc(model, del)
			require.Equal(t, want, visited, "step %d: deleting the current key visits every key once", step)
		case op == 10:
			// Adding keys in a loop goes over the keys collected first, as Set's doc says.
			for _, k := range slices.Collect(m.Keys()) {
				if k%4 == 0 && !m.Has(k+1) {
					m.Set(k+1, step)
					model[k+1] = step
				}
			}
		case r.IntN(20) == 0:
			m.Clear()
			clear(model)
		}

		keys, vals := entries(m)
		wantKeys := slices.Sorted(maps.Keys(model))
		require.Equal(t, len(wantKeys), m.Len(), "step %d", step)
		require.True(t, slices.Equal(wantKeys, keys), "keys at step %d", step)
		for j, k := range wantKeys {
			require.Equal(t, model[k], vals[j], "value for %d at step %d", k, step)
			require.True(t, m.Has(k), "Has(%d) at step %d", k, step)
		}

		if r.IntN(10) == 0 {
			copies = append(copies, m)[max(0, len(copies)-4):]
		}
		for i, c := range copies {
			requireValidMap(t, c, fmt.Sprintf("copy %d at step %d", i, step))
		}
	}
}

// requireValidMap checks what every Map must hold however it was edited: keys strictly ascending,
// each one found by Get, and a MessagePack round trip that gives back the same entries.
func requireValidMap[K immutable.Key, V any](t *testing.T, m immutable.Map[K, V], msg string) {
	t.Helper()
	keys, vals := entries(m)
	ascending := slices.IsSorted(keys) && len(slices.Compact(slices.Clone(keys))) == len(keys)
	require.True(t, ascending, "%s: keys %v are not strictly ascending", msg, keys)
	for _, k := range keys {
		require.True(t, m.Has(k), "%s: Has(%v)", msg, k)
	}
	data, err := msgpack.Marshal(m)
	require.NoError(t, err, msg)
	var back immutable.Map[K, V]
	require.NoError(t, msgpack.Unmarshal(data, &back), "%s: the encoding restores", msg)
	backKeys, backVals := entries(back)
	require.Equal(t, keys, backKeys, msg)
	require.Equal(t, vals, backVals, msg)
}

// TestMap_EncodingIgnoresInsertOrder is the determinism guarantee: the same entries, however they
// were built, encode to the same bytes.
func TestMap_EncodingIgnoresInsertOrder(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	want := map[string]memory{}
	for i := range 50 {
		want[string(rune('a'+i%26))+string(rune('a'+i/26))] = memory{Affinity: i, Seen: immutable.SliceOf("p")}
	}
	keys := slices.Collect(maps.Keys(want))

	var firstPack, firstJSON []byte
	for range 20 {
		r.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
		var m immutable.Map[string, memory]
		for _, k := range keys {
			m.Set(k, want[k])
		}
		pack, err := msgpack.Marshal(m)
		require.NoError(t, err)
		js, err := m.MarshalJSON()
		require.NoError(t, err)
		if firstPack == nil {
			firstPack, firstJSON = pack, js
			continue
		}
		require.Equal(t, firstPack, pack)
		require.True(t, bytes.Equal(firstJSON, js), "JSON: %s vs %s", firstJSON, js)
	}
}

func TestMap_MessagePackRoundTrip(t *testing.T) {
	t.Parallel()

	type holder struct {
		Players immutable.Map[playerID, memory]
		Nested  immutable.Map[string, immutable.Map[int8, string]]
		Empty   immutable.Map[string, int] `msgpack:",omitempty"`
		Zero    immutable.Map[string, int]
	}
	var want holder
	want.Players.Set(7, memory{Affinity: 3, Seen: immutable.SliceOf("a", "b")})
	want.Players.Set(2, memory{Affinity: -1})
	want.Nested.Set("x", mapOf(map[int8]string{-3: "neg", 4: "pos"}))
	data, err := msgpack.Marshal(want)
	require.NoError(t, err)

	var got holder
	require.NoError(t, msgpack.Unmarshal(data, &got))
	assert.True(t, reflect.DeepEqual(want, got))

	// The wire form is a plain map, readable without this package.
	var wire map[string]msgpack.RawMessage
	require.NoError(t, msgpack.Unmarshal(data, &wire))
	assert.NotContains(t, wire, "Empty", "omitempty leaves an empty Map out")
	assert.Equal(t, msgpack.RawMessage{0x80}, wire["Zero"], "the zero value is an empty map, not nil")
	var players map[uint64]map[string]any
	require.NoError(t, msgpack.Unmarshal(wire["Players"], &players))
	assert.Equal(t, []uint64{2, 7}, slices.Sorted(maps.Keys(players)))
}

func TestMap_MessagePackDecodeSortsAndRejectsDuplicates(t *testing.T) {
	t.Parallel()

	encode := func(pairs ...any) []byte {
		data, err := msgpack.Marshal(pairs)
		require.NoError(t, err)
		// Rewrite the array header as a map header of half the length: fixarray 0x9N -> fixmap 0x8N.
		data[0] = 0x80 | byte(len(pairs)/2)
		return data
	}

	var m immutable.Map[uint32, string]
	require.NoError(t, msgpack.Unmarshal(encode(uint32(3), "c", uint32(1), "a", uint32(2), "b"), &m))
	keys, vals := entries(m)
	assert.Equal(t, []uint32{1, 2, 3}, keys, "unsorted input is sorted")
	assert.Equal(t, []string{"a", "b", "c"}, vals)

	for name, data := range map[string][]byte{
		"adjacent duplicate": encode(uint32(1), "a", uint32(1), "b"),
		"spread duplicate":   encode(uint32(2), "a", uint32(1), "b", uint32(2), "c"),
		"truncated":          encode(uint32(1), "a", uint32(2), "b")[:5],
		"wrong key type":     encode("x", "a"),
	} {
		before := m
		require.Error(t, msgpack.Unmarshal(data, &m), name)
		assert.True(t, reflect.DeepEqual(before, m), "%s: a failed decode leaves the Map unchanged", name)
	}

	require.NoError(t, msgpack.Unmarshal([]byte{0xc0}, &m))
	assert.True(t, reflect.DeepEqual(immutable.Map[uint32, string]{}, m), "nil decodes to the zero value")
}

func TestMap_JSON(t *testing.T) {
	t.Parallel()

	ints := mapOf(map[int16]string{10: "ten", -2: "minus two", 3: "three"})
	strs := mapOf(map[string]immutable.Slice[int]{"b": immutable.SliceOf(1), "a": {}})

	for _, lib := range jsonLibs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()

			data, err := lib.marshal(ints)
			require.NoError(t, err)
			assert.JSONEq(t, `{"-2":"minus two","3":"three","10":"ten"}`, string(data))
			assert.True(t, bytes.HasPrefix(data, []byte(`{"-2"`)) && bytes.HasSuffix(data, []byte(`"10":"ten"}`)),
				"entries are in key order: %s", data)
			var backInts immutable.Map[int16, string]
			require.NoError(t, lib.unmarshal(data, &backInts))
			assert.True(t, immutable.MapEqual(ints, backInts))

			data, err = lib.marshal(strs)
			require.NoError(t, err)
			assert.JSONEq(t, `{"a":[],"b":[1]}`, string(data))
			var backStrs immutable.Map[string, immutable.Slice[int]]
			require.NoError(t, lib.unmarshal(data, &backStrs))
			assert.True(t, reflect.DeepEqual(strs, backStrs))

			data, err = lib.marshal(immutable.Map[string, int]{})
			require.NoError(t, err)
			assert.Equal(t, `{}`, string(data), "the zero value is an empty object, not null")

			var unsorted immutable.Map[uint8, bool]
			require.NoError(t, lib.unmarshal([]byte(`{"9":true,"1":false}`), &unsorted))
			keys, _ := entries(unsorted)
			assert.Equal(t, []uint8{1, 9}, keys)

			var cleared = ints
			require.NoError(t, lib.unmarshal([]byte(`null`), &cleared))
			assert.Zero(t, cleared.Len())
		})
	}
}

func TestMap_JSONRejectsBadInput(t *testing.T) {
	t.Parallel()

	for name, input := range map[string]string{
		"duplicate":         `{"1":"a","1":"b"}`,
		"leading zero":      `{"01":"a"}`,
		"plus sign":         `{"+1":"a"}`,
		"not an integer":    `{"x":"a"}`,
		"out of range":      `{"128":"a"}`,
		"array":             `["a"]`,
		"wrong value":       `{"1":2}`,
		"truncated":         `{"1":"a"`,
		"negative zero":     `{"-0":"a"}`,
		"canonical then 05": `{"5":"a","05":"b"}`,
	} {
		for _, lib := range jsonLibs {
			m := mapOf(map[int8]string{1: "keep"})
			require.Error(t, lib.unmarshal([]byte(input), &m), "%s via %s", name, lib.name)
			v, _ := m.Get(1)
			assert.Equal(t, "keep", v, "%s via %s: a failed decode leaves the Map unchanged", name, lib.name)
		}
	}
}

func TestMapTypes(t *testing.T) {
	t.Parallel()

	k, v, ok := immutable.MapTypes(reflect.TypeFor[immutable.Map[playerID, memory]]())
	require.True(t, ok)
	assert.Equal(t, reflect.TypeFor[playerID](), k)
	assert.Equal(t, reflect.TypeFor[memory](), v)

	type Map[K comparable, V any] struct{ keys []K } //nolint:unused // Only its type is inspected.
	for _, typ := range []reflect.Type{
		nil,
		reflect.TypeFor[map[int]int](),
		reflect.TypeFor[immutable.Slice[int]](),
		reflect.TypeFor[Map[int, int]](),
	} {
		_, _, found := immutable.MapTypes(typ)
		assert.False(t, found, "%v", typ)
	}
}

func FuzzMap_DecodeMsgpack(f *testing.F) {
	seed, err := msgpack.Marshal(mapOf(map[int]immutable.Slice[string]{1: immutable.SliceOf("a")}))
	require.NoError(f, err)
	f.Add(seed)
	f.Add([]byte{0x82, 0x01, 0x90, 0x01, 0x90})
	f.Add([]byte{0xdf, 0xff, 0xff, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		var m immutable.Map[int, immutable.Slice[string]]
		if msgpack.Unmarshal(data, &m) != nil {
			return
		}
		keys, _ := entries(m)
		for i := 1; i < len(keys); i++ {
			require.Less(t, keys[i-1], keys[i], "decoded keys are strictly ascending")
		}
		again, err := msgpack.Marshal(m)
		require.NoError(t, err)
		var back immutable.Map[int, immutable.Slice[string]]
		require.NoError(t, msgpack.Unmarshal(again, &back))
		require.True(t, reflect.DeepEqual(m, back))
	})
}

func TestMap_Allocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	m := immutable.MapFrom(maps.All(map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 7: 7, 8: 8}))
	var sink int

	reads := testing.AllocsPerRun(100, func() {
		v, _ := m.Get(5)
		sink += v + m.Len()
		if m.Has(3) {
			sink++
		}
		for k, v := range m.All() {
			sink += k + v
		}
	})
	assert.Zero(t, reads, "reads never allocate")

	assert.Zero(t, testing.AllocsPerRun(100, func() { m.Set(3, 9) }), "Set on an existing key")
	assert.Zero(t, testing.AllocsPerRun(100, func() { m.Delete(30) }), "Delete of an absent key")
	assert.Zero(t, testing.AllocsPerRun(100, func() { m.DeleteFunc(func(int, int) bool { return false }) }),
		"DeleteFunc that removes nothing")
	assert.InDelta(t, 6, testing.AllocsPerRun(100, func() {
		d := m.Clone()
		d.Delete(4)
		d.DeleteFunc(func(k, _ int) bool { return k%2 == 0 })
	}), 0, "Clone, Delete and DeleteFunc: one new pair of arrays each")
	assert.InDelta(t, 2, testing.AllocsPerRun(100, func() { sink += m.Clone().Len() }), 0, "Clone: one per array")

	// Set grows the arrays only when they are full, so 1024 new keys allocate about 2·log2(1024)
	// times, not 2048.
	loop := testing.AllocsPerRun(100, func() {
		var b immutable.Map[int, int]
		for k := range 1024 {
			b.Set(k, k)
		}
		sink += b.Len()
	})
	assert.LessOrEqual(t, loop, 40.0, "1024 new keys")
	_ = sink
}
