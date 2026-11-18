package immutable_test

import (
	"bytes"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

type (
	meters float32
	label  string
)

// encodeItem has a field of every kind a component element may hold.
type encodeItem struct {
	I8    int8
	I16   int16
	I32   int32
	I64   int64
	I     int
	U8    uint8
	U16   uint16
	U32   uint32
	U64   uint64
	U     uint
	F32   float32
	F64   float64
	M     meters
	B     bool
	S     string
	L     label
	A     [3]int16
	D     time.Duration
	T     time.Time
	Inner immutable.Slice[int]
	Tags  immutable.Map[string, int]
	Opt   int                  `msgpack:",omitempty"`
	OptS  immutable.Slice[int] `msgpack:",omitempty"`
}

// The encoders write each element through EncodeValue rather than Encode, to avoid an allocation
// per element. Snapshots and determinism depend on the bytes, so they must be exactly what encoding
// each element with Encode gives, for every kind of element and with any encoder settings.
func TestEncodeMsgpack_SameBytesAsEncodingEachElement(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	ints := []int64{0, 1, -1, 127, 128, -32, -33, 255, 256, math.MaxInt32, math.MinInt32, math.MaxInt64, math.MinInt64}
	floats := []float64{0, math.Copysign(0, -1), 1.5, -2.25, math.NaN(), math.Inf(1), math.Inf(-1), 1e300}
	items := []encodeItem{{}} // all zero, so the omitempty fields are left out
	for range 300 {
		i, f := ints[r.IntN(len(ints))], floats[r.IntN(len(floats))]
		items = append(items, encodeItem{
			I8: int8(i), I16: int16(i), I32: int32(i), I64: i, I: int(i),
			U8: uint8(i), U16: uint16(i), U32: uint32(i), U64: uint64(i), U: uint(i),
			F32: float32(f), F64: f, M: meters(f), B: i%2 == 0, S: string(rune('a' + r.IntN(26))),
			L: label("x"), A: [3]int16{int16(i), 2, 3}, D: time.Duration(i), T: time.Unix(i%1e9, i%1e9).UTC(),
			Inner: immutable.SliceOf(int(i), 2), Tags: immutable.MapFrom(func(yield func(string, int) bool) {
				yield("k", int(i))
			}),
			Opt: int(i), OptS: immutable.SliceOf(1),
		})
	}

	settings := map[string]func(*msgpack.Encoder){
		"default": func(*msgpack.Encoder) {},
		"compact": func(e *msgpack.Encoder) {
			e.UseCompactInts(true)
			e.UseCompactFloats(true)
		},
	}
	for name, set := range settings {
		requireSameSliceBytes(t, name, set, immutable.SliceOf(items...))
		requireSameSliceBytes(t, name, set, immutable.SliceOf(ints...))
		requireSameSliceBytes(t, name, set, immutable.SliceOf(floats...))
		requireSameSliceBytes(t, name, set, immutable.SliceOf[float32](1.5, float32(math.NaN()), -0.25))
		requireSameSliceBytes(t, name, set, immutable.SliceOf[meters](1, 2))
		requireSameSliceBytes(t, name, set, immutable.SliceOf("a", "", "long string over thirty-one bytes!"))
		requireSameSliceBytes(t, name, set, immutable.SliceOf(time.Unix(5, 6).UTC(), time.Time{}))
		requireSameSliceBytes(t, name, set, immutable.SliceOf(time.Second, -time.Minute))

		requireSameMapBytes(t, name, set, immutable.MapFrom(func(yield func(int, encodeItem) bool) {
			for i, it := range items {
				if !yield(i*7-1000, it) {
					return
				}
			}
		}))
		requireSameMapBytes(t, name, set, immutable.MapFrom(func(yield func(label, float32) bool) {
			for i, f := range floats {
				if !yield(label(rune('a'+i)), float32(f)) {
					return
				}
			}
		}))
		requireSameMapBytes(t, name, set, immutable.MapFrom(func(yield func(uint8, time.Duration) bool) {
			for i := range 200 {
				if !yield(uint8(i), time.Duration(i)*time.Millisecond) {
					return
				}
			}
		}))
	}
}

func requireSameSliceBytes[T any](t *testing.T, name string, set func(*msgpack.Encoder), s immutable.Slice[T]) {
	t.Helper()
	var want, got bytes.Buffer
	oldEnc := msgpack.NewEncoder(&want)
	set(oldEnc)
	require.NoError(t, oldEnc.EncodeArrayLen(s.Len()))
	for v := range s.Values() {
		require.NoError(t, oldEnc.Encode(v))
	}
	enc := msgpack.NewEncoder(&got)
	set(enc)
	require.NoError(t, enc.Encode(s))
	require.Equal(t, want.Bytes(), got.Bytes(), "%s: Slice[%T]", name, *new(T))
}

func requireSameMapBytes[K immutable.Key, V any](
	t *testing.T, name string, set func(*msgpack.Encoder), m immutable.Map[K, V],
) {
	t.Helper()
	var want, got bytes.Buffer
	oldEnc := msgpack.NewEncoder(&want)
	set(oldEnc)
	require.NoError(t, oldEnc.EncodeMapLen(m.Len()))
	for k, v := range m.All() {
		require.NoError(t, oldEnc.Encode(k))
		require.NoError(t, oldEnc.Encode(v))
	}
	enc := msgpack.NewEncoder(&got)
	set(enc)
	require.NoError(t, enc.Encode(m))
	require.Equal(t, want.Bytes(), got.Bytes(), "%s: Map[%T]%T", name, *new(K), *new(V))
}

// Encoding allocates once for the whole collection, not once per element.
func TestEncodeMsgpack_Allocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	items := make([]point, 4096)
	for i := range items {
		items[i] = point{i, -i}
	}
	s := immutable.SliceOf(items...)
	m := immutable.MapFrom(func(yield func(int, point) bool) {
		for i, p := range items {
			if !yield(i, p) {
				return
			}
		}
	})
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	encode := func(v any) func() {
		return func() {
			buf.Reset()
			require.NoError(t, enc.Encode(v))
		}
	}
	encode(s)() // grow the buffer once, so the counts below are the encoder's own
	encode(m)()
	require.LessOrEqual(t, testing.AllocsPerRun(20, encode(s)), 4.0, "encoding 4096 elements")
	require.LessOrEqual(t, testing.AllocsPerRun(20, encode(m)), 4.0, "encoding 4096 entries")
}

// A DeleteFunc that keeps a few elements of many must not keep the old array alive through them.
func TestDeleteFunc_DropsTheLargeArray(t *testing.T) {
	if raceEnabled {
		t.Skip("heap measurements are not meaningful under -race")
	}
	type blob [64]byte
	heap := func() int64 {
		runtime.GC()
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return int64(ms.HeapAlloc)
	}
	keepTen := func(i int) bool { return i >= 10 }

	before := heap()
	items := make([]blob, 4096)
	for i := range items {
		items[i][0] = byte(i % 251)
		items[i][1] = byte(i / 251)
	}
	s := immutable.SliceOf(items...)
	n := 0
	s.DeleteFunc(func(blob) bool {
		n++
		return keepTen(n - 1)
	})
	slice := heap() - before
	require.Equal(t, 10, s.Len())
	runtime.KeepAlive(s)
	require.Less(t, slice, int64(64<<10), "ten 64-byte elements must not hold the 256 KB array")

	before = heap()
	m := immutable.MapFrom(func(yield func(int, blob) bool) {
		for i := range 4096 {
			if !yield(i, blob{}) {
				return
			}
		}
	})
	m.DeleteFunc(func(k int, _ blob) bool { return keepTen(k) })
	mapped := heap() - before
	require.Equal(t, 10, m.Len())
	runtime.KeepAlive(m)
	require.Less(t, mapped, int64(64<<10), "ten entries must not hold the 290 KB array")
}
