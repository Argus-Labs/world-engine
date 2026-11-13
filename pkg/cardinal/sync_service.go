package cardinal

import (
	"context"
	"encoding/binary"
	"math"
	"math/bits"
	"slices"
	"strings"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
	"google.golang.org/protobuf/proto"
)

// The log items queued as event.KindSync, in order with the tick's other events.
type (
	// syncCommit is one capture's transitions.
	syncCommit []syncTransition
	// syncOpen starts a stream for a reader: a baseline of every current entry it may view.
	syncOpen struct {
		slot       int
		subscriber *streamSubscriber
		generation uint64
	}
	// syncClose ends a reader's stream. A reader still subscribed is told to drop its view.
	syncClose struct {
		slot  int
		clear bool
	}
)

// reconcileSync runs after the tick's last capture. It reads the subscriptions once, under the
// service lock, and queues an open or close for every reader whose stream must change. Opens and
// closes follow the tick's commits, so a baseline reads the entries directly.
func (w *World) reconcileSync() {
	p := w.sync
	if !p.enabled() {
		return
	}
	type stream struct {
		player     string
		subscriber *streamSubscriber
		generation uint64
	}
	s := w.service
	s.mu.RLock()
	subscribed := make([]stream, 0, len(s.subscribers))
	for player, sub := range s.subscribers {
		if sub.wantsSync() {
			subscribed = append(subscribed, stream{player, sub, sub.syncGeneration})
		}
	}
	s.mu.RUnlock()
	slices.SortFunc(subscribed, func(a, b stream) int { return strings.Compare(a.player, b.player) })

	want := map[string]stream{}
	revoked := map[string]stream{}
	for _, c := range subscribed {
		if p.admit(c.player) {
			want[c.player] = c
		} else {
			revoked[c.player] = c
		}
	}
	same := func(r syncReader, c stream, ok bool) bool {
		return ok && c.subscriber == r.subscriber && c.generation == r.generation
	}
	for slot, r := range p.roster.readers {
		if !p.roster.live.has(slot) {
			continue
		}
		if c, ok := want[r.player]; same(r, c, ok) {
			continue
		}
		c, ok := revoked[r.player]
		p.roster.leave(r.player)
		w.events.Enqueue(event.Event{Kind: event.KindSync, Payload: syncClose{slot: slot, clear: same(r, c, ok)}})
	}
	for _, c := range subscribed {
		if _, ok := want[c.player]; !ok {
			continue
		}
		if slot, seated := p.roster.slotOf[c.player]; seated && p.roster.live.has(slot) &&
			same(p.roster.readers[slot], c, true) {
			continue
		}
		slot := p.seat(c.player)
		w.events.Enqueue(event.Event{Kind: event.KindSync, Payload: syncOpen{slot, c.subscriber, c.generation}})
	}
}

// wantsSync reports whether the stream asks for world_sync. A cancelled stream does not, even
// while its handler is still blocked writing to it: nothing more reaches that client, so its reader
// leaves its seat. Requires service.mu.
func (s *streamSubscriber) wantsSync() bool {
	_, ok := s.events[syncstate.EventName]
	return ok && s.ctx.Err() == nil
}

// dispatchSync handles one sync log item, in order with the tick's other events. It reads only
// the log and the publisher, never the live world.
func (w *World) dispatchSync(_ context.Context, evt event.Event) error {
	p := w.sync
	switch item := evt.Payload.(type) {
	case syncCommit:
		deltas := p.emptyDeltas()
		p.fold(item, deltas)
		for slot := range deltas {
			if !deltas[slot].empty() {
				w.sendSync(&p.roster.readers[slot], syncContinues, deltas[slot])
			}
		}
	case syncOpen:
		r := &p.roster.readers[item.slot]
		r.subscriber, r.generation, r.stream, r.sequence = item.subscriber, item.generation, p.nextStream(), 0
		p.roster.live.put(item.slot, true)
		w.sendSync(r, syncOpens, p.baseline(item.slot))
	case syncClose:
		r := &p.roster.readers[item.slot]
		p.roster.live.put(item.slot, false)
		if item.clear {
			r.stream, r.sequence = p.nextStream(), 0
			w.sendSync(r, syncClears, syncDelta{})
		}
		r.subscriber = nil
	}
	return nil
}

// settleSync ends the tick's publication: open readers due a heartbeat get one, slots freed
// during the tick become reusable, policies no entry uses are dropped, and development builds
// check the derived state.
func (w *World) settleSync() {
	p := w.sync
	if !p.enabled() {
		return
	}
	w.heartbeatSync()
	p.roster.release()
	clear(p.retired)
	p.sweep()
	p.check()
}

// heartbeatMillis is sync.Heartbeat as Open.Heartbeat carries it.
const heartbeatMillis = uint64(syncstate.Heartbeat / time.Millisecond)

// heartbeatSync keeps the promise of sync.Heartbeat once the tick's log has been dispatched: an
// open reader whose last frame is a heartbeat or more old gets an empty delta. A reader that got
// a frame this tick is never due.
func (w *World) heartbeatSync() {
	p := w.sync
	now := w.syncTime()
	for word, open := range p.roster.live {
		for m := open; m != 0; m &= m - 1 {
			r := &p.roster.readers[word<<6+bits.TrailingZeros64(m)]
			if now >= r.clock+heartbeatMillis {
				w.sendSync(r, syncContinues, syncDelta{})
			}
		}
	}
}

// syncTime is the running tick's time in Unix milliseconds: what every frame's Time counts.
func (w *World) syncTime() uint64 {
	return uint64(max(0, w.Timestamp().UnixMilli()))
}

// nextStream numbers a new stream. Numbers are never reused while the world runs, so a delta
// from a stream the reader left never matches the one it holds.
func (p *syncPublisher) nextStream() uint64 {
	p.streams++
	return p.streams
}

// syncStart is how a frame stands to the reader's stream.
type syncStart uint8

const (
	syncContinues syncStart = iota // a delta on the open stream
	syncOpens                      // a baseline for a seated reader, promised a heartbeat
	syncClears                     // a baseline that clears a revoked reader's view: no promise
)

// sendSync numbers and times one frame on the reader's stream and queues it for the reader's
// writer. A baseline opens the stream: it alone says the protocol, the contract, the session and
// the heartbeat, and its Time restarts the stream's clock. A clearing baseline promises no
// heartbeat, since its reader is no longer seated. A delta's Time is what the clock moved since
// the last frame; if the tick's time went back, it is 0 until the time catches up.
func (w *World) sendSync(r *syncReader, start syncStart, d syncDelta) {
	baseline := start != syncContinues
	r.sequence++
	if baseline {
		r.clock = 0
	}
	now := w.syncTime()
	elapsed := now - min(now, r.clock)
	r.clock += elapsed
	if r.subscriber == nil || r.subscriber.ctx.Err() != nil {
		return
	}
	var open []byte
	if baseline {
		var heartbeat uint64
		if start == syncOpens {
			heartbeat = heartbeatMillis
		}
		p := w.sync
		if p.fingerprint == "" {
			fingerprint, err := w.SyncContract().Fingerprint()
			if err != nil {
				panic(err)
			}
			p.fingerprint = fingerprint
		}
		open = encodeSync(syncstate.Open{
			Version: syncstate.ProtocolVersion, Fingerprint: p.fingerprint, Session: w.world.Session(),
			Heartbeat: heartbeat,
		})
	}
	payload := appendFrame(nil, open, r.stream, r.sequence, elapsed, d)
	evt := &iscv1.Event{Name: syncstate.EventName, Payload: payload}
	// A full queue ends the stream as lagged; the next reconcile closes its slot.
	_ = r.subscriber.enqueue(&cardinalv1.StartEventStreamResponse{Address: w.address, Event: evt}, proto.Size(evt))
}

// emptyDeltas returns an empty delta for every slot. Their lists keep their capacity, and the
// encodings they last held until overwritten, from earlier commits: sendSync copies a delta into
// its frame, so no delta is read after its commit.
func (p *syncPublisher) emptyDeltas() []syncDelta {
	for len(p.deltas) < len(p.roster.readers) {
		p.deltas = append(p.deltas, syncDelta{})
	}
	deltas := p.deltas[:len(p.roster.readers)]
	for i := range deltas {
		d := &deltas[i]
		d.records, d.patches, d.removed, d.access = d.records[:0], d.patches[:0], d.removed[:0], d.access[:0]
	}
	return deltas
}

// appendFrame appends one reader's frame to b, byte for byte what encodeSync makes of the
// sync.Frame it stands for: an array in field order and the smallest integer encodings. An empty
// list is written as nil, as the engine always writes it. Records and patches are copied as
// encoded once at capture. open is the encoded Open of a baseline, nil on a delta; elapsed is the
// frame's Time.
func appendFrame(b []byte, open []byte, stream, sequence, elapsed uint64, d syncDelta) []byte {
	const refMax, grantMax = 1 + 5 + 9, 1 + (1 + 5 + 9) + 1 // [entity, incarnation], [ref, access]
	size := 1 + max(1, len(open)) + 3*9 + 4*5 + len(d.removed)*refMax + len(d.access)*grantMax
	for _, raw := range d.records {
		size += len(raw)
	}
	for _, raw := range d.patches {
		size += len(raw)
	}
	b = slices.Grow(b, size)

	b = append(b, msgpcode.FixedArrayLow|8) // the frame's eight fields
	if open == nil {
		b = append(b, msgpcode.Nil)
	} else {
		b = append(b, open...)
	}
	b = appendUint(b, stream)
	b = appendUint(b, sequence)
	b = appendUint(b, elapsed)
	b = appendRaw(b, d.records)
	b = appendRaw(b, d.patches)
	b = appendArrayLen(b, len(d.removed))
	for _, ref := range d.removed {
		b = appendRef(b, ref)
	}
	b = appendArrayLen(b, len(d.access))
	for _, g := range d.access {
		b = appendUint(appendRef(append(b, msgpcode.FixedArrayLow|2), g.Ref), uint64(g.Access))
	}
	return b
}

func appendRaw(b []byte, list []msgpack.RawMessage) []byte {
	b = appendArrayLen(b, len(list))
	for _, raw := range list {
		b = append(b, raw...)
	}
	return b
}

func appendRef(b []byte, ref syncstate.Ref) []byte {
	return appendUint(appendUint(append(b, msgpcode.FixedArrayLow|2), uint64(ref.Entity)), ref.Incarnation)
}

// appendArrayLen appends a list's array header, or nil for an empty list.
func appendArrayLen(b []byte, n int) []byte {
	switch {
	case n == 0:
		return append(b, msgpcode.Nil)
	case n < 16:
		return append(b, msgpcode.FixedArrayLow|byte(n)) //nolint:gosec // 0 < n < 16
	case n <= math.MaxUint16:
		return binary.BigEndian.AppendUint16(append(b, msgpcode.Array16), uint16(n))
	}
	// A frame's list is far shorter than 2^32.
	return binary.BigEndian.AppendUint32(append(b, msgpcode.Array32), uint32(n)) //nolint:gosec // see above
}

// appendUint appends n in its smallest MessagePack encoding, as UseCompactInts does.
func appendUint(b []byte, n uint64) []byte {
	switch {
	case n <= math.MaxInt8:
		return append(b, byte(n))
	case n <= math.MaxUint8:
		return append(b, msgpcode.Uint8, byte(n))
	case n <= math.MaxUint16:
		return binary.BigEndian.AppendUint16(append(b, msgpcode.Uint16), uint16(n))
	case n <= math.MaxUint32:
		return binary.BigEndian.AppendUint32(append(b, msgpcode.Uint32), uint32(n))
	}
	return binary.BigEndian.AppendUint64(append(b, msgpcode.Uint64), n)
}

// syncAudience narrows an event's recipients by sync access, judged at dispatch.
type syncAudience struct {
	publisher  *syncPublisher
	id         syncstate.ID // the lifetime the event is about, when stamped
	stamped    bool
	occurrence bool        // live feedback: only for readers whose sync stream is open
	ungated    bool        // the lifetime was alive and not synced when the event was emitted
	fallback   *syncPolicy // judged by this if the lifetime has no entry at dispatch; nil: nobody
}

// audienceOf returns the event's sync audience; nil admits everyone.
func audienceOf(evt event.Event) *syncAudience {
	audience, _ := evt.Audience.(*syncAudience)
	return audience
}

// policy is what the event is judged by: the lifetime's entry if it has one now, else the
// fallback captured at emit. It returns false when access does not apply.
func (a *syncAudience) policy() (*syncPolicy, bool) {
	if !a.stamped {
		return nil, false
	}
	if e := a.publisher.entry(EntityID(a.id.Entity)); e != nil && e.id == a.id {
		return e.policy, true
	}
	return a.fallback, !a.ungated
}

// admits reports whether player may receive the event on subscriber, or as a reply waiter when
// subscriber is nil. A nil audience admits everyone. An occurrence needs the player's open sync
// stream; any stamped event needs view access to its entity, and admission.
func (a *syncAudience) admits(player string, subscriber *streamSubscriber) bool {
	if a == nil {
		return true
	}
	p := a.publisher
	pol, gated := a.policy()
	if a.occurrence {
		slot, seated := p.roster.slotOf[player]
		if !seated || !p.roster.live.has(slot) ||
			(subscriber != nil && p.roster.readers[slot].subscriber != subscriber) {
			return false
		}
		return !gated || p.access(pol, player) >= syncstate.View
	}
	return !gated || (p.access(pol, player) >= syncstate.View && p.admit(player))
}
