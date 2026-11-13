package cardinal

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// reflectFrame is the frame wrapper as the engine encoded it through reflection before frames
// were appended by hand: sync.Frame's fields in order, with records and patches already encoded.
type reflectFrame struct {
	_msgpack struct{} `msgpack:",as_array"` //nolint:unused // msgpack reads the tag: encode as an array

	Open     *syncstate.Open
	Stream   uint64
	Sequence uint64
	Records  []msgpack.RawMessage
	Patches  []msgpack.RawMessage
	Removed  []syncstate.Ref
	Access   []syncstate.Grant
}

// Every frame appendFrame writes is byte for byte the reflective encoding of the same frame, at
// every integer width and list-length boundary the compact layout distinguishes. An empty list,
// nil or not, is written as nil: the engine always writes nil.
func TestSyncAppendFrameMatchesReflectiveEncoding(t *testing.T) {
	widths := []uint64{0, 1, 127, 128, 255, 256, 65535, 65536, math.MaxUint32, math.MaxUint32 + 1, math.MaxUint64}
	lengths := []int{0, 1, 15, 16, 17, 65535, 65536}
	rng := rand.New(rand.NewPCG(1, 2))
	num := func() uint64 { return widths[rng.IntN(len(widths))] }
	ref := func() syncstate.Ref {
		return syncstate.Ref{Entity: uint32(num()), Incarnation: num()}
	}
	raw := func() msgpack.RawMessage {
		return encodeSync(syncstate.Patch{Ref: ref(), Revision: num(), Dropped: []uint16{uint16(rng.Uint32())}})
	}
	list := func(n int, nilWhenEmpty bool) int {
		if n == 0 && nilWhenEmpty {
			return -1
		}
		return n
	}
	for i := range 400 {
		var d syncDelta
		n := func() int {
			if rng.IntN(8) == 0 {
				return lengths[rng.IntN(len(lengths))]
			}
			return rng.IntN(20)
		}
		if k := list(n(), rng.IntN(2) == 0); k >= 0 {
			d.records = make([]msgpack.RawMessage, 0, k)
			for range k {
				d.records = append(d.records, raw())
			}
		}
		if k := list(n(), rng.IntN(2) == 0); k >= 0 {
			d.patches = make([]msgpack.RawMessage, 0, k)
			for range k {
				d.patches = append(d.patches, raw())
			}
		}
		if k := list(n(), rng.IntN(2) == 0); k >= 0 {
			d.removed = make([]syncstate.Ref, 0, k)
			for range k {
				d.removed = append(d.removed, ref())
			}
		}
		if k := list(n(), rng.IntN(2) == 0); k >= 0 {
			d.access = make([]syncstate.Grant, 0, k)
			for range k {
				access := syncstate.Access(rng.IntN(3))
				d.access = append(d.access, syncstate.Grant{Ref: ref(), Access: access})
			}
		}
		want := reflectFrame{
			Stream: num(), Sequence: num(),
			Records: nilIfEmpty(d.records), Patches: nilIfEmpty(d.patches),
			Removed: nilIfEmpty(d.removed), Access: nilIfEmpty(d.access),
		}
		var open []byte
		if rng.IntN(4) == 0 {
			want.Open = &syncstate.Open{
				Version: syncstate.ProtocolVersion, Fingerprint: fmt.Sprintf("%064x", i), Session: "session",
			}
			open = encodeSync(*want.Open)
		}
		got := appendFrame(nil, open, want.Stream, want.Sequence, d)
		require.Equal(t, encodeSync(want), got, "frame %d", i)
	}
}

func nilIfEmpty[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}

// BenchmarkSyncFrameAssembly is the limit preset's per-reader frame: 2,600 transform patches.
func BenchmarkSyncFrameAssembly(b *testing.B) {
	data, err := msgpack.Marshal(map[string]float64{"X": 123.456, "Y": 654.321})
	require.NoError(b, err)
	var d syncDelta
	for n := range 2600 {
		d.patches = append(d.patches, encodeSync(syncstate.Patch{
			Ref: syncstate.Ref{Entity: uint32(n), Incarnation: 1}, Revision: 1000,
			Changed: []syncstate.Value{{Component: 3, Presence: 1, Data: data}},
		}))
	}
	b.Run("reflect", func(b *testing.B) {
		for b.Loop() {
			_ = encodeSync(reflectFrame{Stream: 7, Sequence: 9000, Patches: d.patches})
		}
	})
	b.Run("append", func(b *testing.B) {
		for b.Loop() {
			_ = appendFrame(nil, nil, 7, 9000, d)
		}
	})
}

// BenchmarkSyncTickLimit is one tick of the stress harness's limit shape on the tick goroutine:
// 200 open readers that may all view 5,000 entities, 2,500 of which change. It covers capture,
// the fold, frame assembly and queueing; nothing writes the queued frames.
func BenchmarkSyncTickLimit(b *testing.B) {
	const readers, entities, moving = 200, 5000, 2500
	w := NewTestWorld(b, func(w *World) {
		w.RegisterComponent[syncHealth]()
		w.RegisterComponent[syncPrivate]()
		w.EnableSync(func(string) bool { return true })
	})
	var subs []*streamSubscriber
	for i := range readers {
		ctx, cancel := context.WithCancelCause(b.Context())
		sub := &streamSubscriber{
			ctx: ctx, cancel: cancel, queueReady: make(chan struct{}, 1),
			events: map[string]struct{}{syncstate.EventName: {}},
		}
		addSubscriptions(sub, []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}})
		w.service.addSubscriber(fmt.Sprintf("bot-%d", i), sub)
		subs = append(subs, sub)
	}
	var all []Entity
	for range entities {
		e := w.Create[syncAvatar]()
		e.Set(syncHealth{HP: 1, MaxHP: 100})
		all = append(all, e)
	}
	drain := func() {
		for _, sub := range subs {
			sub.queue, sub.queuedBytes = nil, 0
		}
	}
	w.Tick() // baselines
	drain()
	hp := 1
	for b.Loop() {
		hp++
		for _, e := range all[:moving] {
			e.Set(syncHealth{HP: hp, MaxHP: 100})
		}
		w.Tick()
		drain()
	}
}
