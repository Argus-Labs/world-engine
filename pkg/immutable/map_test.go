package immutable_test

import (
	"bytes"
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

	m := immutable.Map[int, string]{}.With(3, "c").With(1, "a").With(2, "b")
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

func TestMap_DerivationsLeaveReceiverUnchanged(t *testing.T) {
	t.Parallel()

	base := mapOf(map[string]int{"a": 1, "b": 2, "c": 3})
	derive := map[string]func(immutable.Map[string, int]) immutable.Map[string, int]{
		"With existing": func(m immutable.Map[string, int]) immutable.Map[string, int] { return m.With("b", 9) },
		"With new":      func(m immutable.Map[string, int]) immutable.Map[string, int] { return m.With("bb", 9) },
		"Without":       func(m immutable.Map[string, int]) immutable.Map[string, int] { return m.Without("b") },
		"Update":        func(m immutable.Map[string, int]) immutable.Map[string, int] { return m.Update("a", inc) },
		"builder set existing": func(m immutable.Map[string, int]) immutable.Map[string, int] {
			b := m.Edit()
			b.Set("a", 7)
			return b.Done()
		},
		"builder insert and delete": func(m immutable.Map[string, int]) immutable.Map[string, int] {
			b := m.Edit()
			b.Set("z", 7)
			b.Delete("a")
			return b.Done()
		},
	}
	for name, f := range derive {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := f(base)
			again := f(got)
			_ = again
			keys, vals := entries(base)
			assert.Equal(t, []string{"a", "b", "c"}, keys, "the receiver's keys are unchanged")
			assert.Equal(t, []int{1, 2, 3}, vals, "the receiver's values are unchanged")
		})
	}
}

func inc(v int) int { return v + 1 }

func TestMap_WithWithoutUpdate(t *testing.T) {
	t.Parallel()

	m := mapOf(map[string]int{"a": 1, "c": 3})
	assert.True(t, immutable.MapEqual(mapOf(map[string]int{"a": 1, "b": 2, "c": 3}), m.With("b", 2)))
	assert.True(t, immutable.MapEqual(mapOf(map[string]int{"a": 5, "c": 3}), m.With("a", 5)))
	assert.True(t, immutable.MapEqual(mapOf(map[string]int{"c": 3}), m.Without("a")))
	assert.True(t, immutable.MapEqual(m, m.Without("zz")), "removing an absent key changes nothing")
	assert.True(t, immutable.MapEqual(mapOf(map[string]int{"a": 2, "c": 3}), m.Update("a", inc)))
	assert.True(t, immutable.MapEqual(mapOf(map[string]int{"a": 1, "c": 3, "d": 1}), m.Update("d", inc)),
		"Update starts from the zero value for an absent key")
}

func TestMap_EmptyIsZeroValue(t *testing.T) {
	t.Parallel()

	one := immutable.Map[string, int]{}.With("a", 1)
	b := one.Edit()
	b.Delete("a")

	var fromMsgpack, fromJSON immutable.Map[string, int]
	require.NoError(t, msgpack.Unmarshal([]byte{0x80}, &fromMsgpack))
	require.NoError(t, fromJSON.UnmarshalJSON([]byte(`{}`)))

	for name, m := range map[string]immutable.Map[string, int]{
		"Without last":  one.Without("a"),
		"builder Done":  b.Done(),
		"MapFrom empty": immutable.MapFrom(maps.All(map[string]int{})),
		"msgpack {}":    fromMsgpack,
		"JSON {}":       fromJSON,
	} {
		assert.True(t, reflect.DeepEqual(immutable.Map[string, int]{}, m), name)
	}
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

	a := immutable.Map[int, memory]{}.With(1, memory{Seen: immutable.SliceOf("x")})
	b := immutable.Map[int, memory]{}.With(1, memory{Seen: immutable.SliceOf("x")})
	eq := func(x, y memory) bool { return x.Affinity == y.Affinity && immutable.Equal(x.Seen, y.Seen) }
	assert.True(t, a.EqualFunc(b, eq))
	assert.False(t, a.EqualFunc(b.With(2, memory{}), eq))
	assert.False(t, a.EqualFunc(b.With(1, memory{Affinity: 1}), eq))
}

func TestMapBuilder_SeesPendingEdits(t *testing.T) {
	t.Parallel()

	src := mapOf(map[playerID]int{1: 10, 2: 20})
	b := src.Edit()
	for _, hit := range []struct {
		who playerID
		dmg int
	}{{1, 5}, {3, 7}, {3, 1}, {2, 0}} {
		v, _ := b.Get(hit.who)
		b.Set(hit.who, v+hit.dmg)
	}
	assert.True(t, b.Has(3))
	b.Delete(2)
	b.Delete(99)
	assert.False(t, b.Has(2))
	assert.Equal(t, 2, b.Len())

	keys, vals := entries(b.Done())
	assert.Equal(t, []playerID{1, 3}, keys)
	assert.Equal(t, []int{15, 8}, vals)
	keys, vals = entries(src)
	assert.Equal(t, []playerID{1, 2}, keys, "the source is unchanged")
	assert.Equal(t, []int{10, 20}, vals, "the source is unchanged")
}

func TestMapBuilder_UseAfterDonePanics(t *testing.T) {
	t.Parallel()

	uses := map[string]func(*immutable.MapBuilder[string, int]){
		"Len":    func(b *immutable.MapBuilder[string, int]) { b.Len() },
		"Get":    func(b *immutable.MapBuilder[string, int]) { b.Get("a") },
		"Has":    func(b *immutable.MapBuilder[string, int]) { b.Has("a") },
		"Set":    func(b *immutable.MapBuilder[string, int]) { b.Set("a", 1) },
		"Delete": func(b *immutable.MapBuilder[string, int]) { b.Delete("a") },
		"Done":   func(b *immutable.MapBuilder[string, int]) { b.Done() },
	}
	for name, use := range uses {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b := immutable.Map[string, int]{}.With("a", 1).Edit()
			done := b.Done()
			require.PanicsWithValue(t, "immutable: MapBuilder used after Done", func() { use(b) })
			v, _ := done.Get("a")
			assert.Equal(t, 1, v, "the built Map is untouched")
		})
	}
}

// TestMap_ModelMatchesGoMap runs random edits, through derivations and builders alike, against a Go
// map read back in sorted key order. Like the Slice model test, it keeps recent values and checks
// after every step that none of them changed.
func TestMap_ModelMatchesGoMap(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	type version struct {
		m    immutable.Map[int, int]
		want map[int]int
	}
	cur := immutable.Map[int, int]{}
	model := map[int]int{}
	var history []version

	for step := range 3000 {
		k, v := r.IntN(40), r.IntN(1000)
		model = maps.Clone(model)
		switch r.IntN(5) {
		case 0, 1:
			cur = cur.With(k, v)
			model[k] = v
		case 2:
			cur = cur.Without(k)
			delete(model, k)
		case 3:
			cur = cur.Update(k, inc)
			model[k]++
		default:
			b := cur.Edit()
			for range r.IntN(6) {
				bk, bv := r.IntN(40), r.IntN(1000)
				if r.IntN(3) == 0 {
					b.Delete(bk)
					delete(model, bk)
					continue
				}
				got, _ := b.Get(bk)
				b.Set(bk, got+bv)
				model[bk] += bv
			}
			cur = b.Done()
		}

		history = append(history, version{m: cur, want: model})
		for i, h := range history {
			keys, vals := entries(h.m)
			wantKeys := slices.Sorted(maps.Keys(h.want))
			require.Equal(t, len(wantKeys), h.m.Len(), "version %d at step %d", i, step)
			require.True(t, slices.Equal(wantKeys, keys), "version %d keys at step %d", i, step)
			for j, k := range wantKeys {
				require.Equal(t, h.want[k], vals[j], "version %d value for %d at step %d", i, k, step)
			}
		}
		if len(history) > 32 {
			history = history[1:]
		}
	}
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
		if r.IntN(2) == 0 {
			for _, k := range keys {
				m = m.With(k, want[k])
			}
		} else {
			b := m.Edit()
			for _, k := range keys {
				b.Set(k, want[k])
			}
			m = b.Done()
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
	}
	want := holder{
		Players: immutable.Map[playerID, memory]{}.
			With(7, memory{Affinity: 3, Seen: immutable.SliceOf("a", "b")}).
			With(2, memory{Affinity: -1}),
		Nested: immutable.Map[string, immutable.Map[int8, string]]{}.
			With("x", immutable.Map[int8, string]{}.With(-3, "neg").With(4, "pos")),
	}
	data, err := msgpack.Marshal(want)
	require.NoError(t, err)

	var got holder
	require.NoError(t, msgpack.Unmarshal(data, &got))
	assert.True(t, reflect.DeepEqual(want, got))

	// The wire form is a plain map, readable without this package.
	var wire map[string]msgpack.RawMessage
	require.NoError(t, msgpack.Unmarshal(data, &wire))
	assert.NotContains(t, wire, "Empty", "omitempty leaves an empty Map out")
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

	ints := immutable.Map[int16, string]{}.With(10, "ten").With(-2, "minus two").With(3, "three")
	strs := immutable.Map[string, immutable.Slice[int]]{}.With("b", immutable.SliceOf(1)).
		With("a", immutable.Slice[int]{})

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
			m := immutable.Map[int8, string]{}.With(1, "keep")
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
	seed, err := msgpack.Marshal(immutable.Map[int, immutable.Slice[string]]{}.With(1, immutable.SliceOf("a")))
	require.NoError(f, err)
	f.Add(seed)
	f.Add([]byte{0x82, 0x01, 0x90, 0x01, 0x90})
	f.Add([]byte{0xdf, 0xff, 0xff, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		var m immutable.Map[int, immutable.Slice[string]]
		if msgpack.Unmarshal(data, &m) != nil {
			return
		}
		for i := 1; i < m.Len(); i++ {
			keys, _ := entries(m)
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

	assert.InDelta(t, 1, testing.AllocsPerRun(100, func() { sink += m.With(3, 9).Len() }), 0, "With existing")
	assert.InDelta(t, 2, testing.AllocsPerRun(100, func() { sink += m.With(30, 9).Len() }), 0, "With new")
	assert.InDelta(t, 2, testing.AllocsPerRun(100, func() { sink += m.Without(3).Len() }), 0, "Without")
	assert.InDelta(t, 0, testing.AllocsPerRun(100, func() { sink += m.Without(30).Len() }), 0, "Without absent")

	// Changing existing entries copies the values once, plus the builder itself.
	batch := testing.AllocsPerRun(100, func() {
		b := m.Edit()
		for k := range 8 {
			v, _ := b.Get(k + 1)
			b.Set(k+1, v*2)
		}
		sink += b.Done().Len()
	})
	assert.LessOrEqual(t, batch, 2.0, "a batch of updates to existing keys")
	_ = sink
}
