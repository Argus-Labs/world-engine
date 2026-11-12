package cardinal

import (
	"context"
	"slices"
	"strings"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
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
		if _, ok := sub.events[syncstate.EventName]; ok {
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

// dispatchSync handles one sync log item, in order with the tick's other events. It reads only
// the log and the publisher, never the live world.
func (w *World) dispatchSync(_ context.Context, evt event.Event) error {
	p := w.sync
	switch item := evt.Payload.(type) {
	case syncCommit:
		deltas := make([]syncDelta, len(p.roster.readers))
		p.fold(item, deltas)
		for slot := range deltas {
			if !deltas[slot].empty() {
				w.sendSync(&p.roster.readers[slot], false, deltas[slot])
			}
		}
	case syncOpen:
		r := &p.roster.readers[item.slot]
		r.subscriber, r.generation, r.stream, r.sequence = item.subscriber, item.generation, uuid.NewString(), 0
		p.roster.live.put(item.slot, true)
		w.sendSync(r, true, p.baseline(item.slot))
	case syncClose:
		r := &p.roster.readers[item.slot]
		p.roster.live.put(item.slot, false)
		if item.clear {
			r.stream, r.sequence = uuid.NewString(), 0
			w.sendSync(r, true, syncDelta{})
		}
		r.subscriber = nil
	}
	return nil
}

// settleSync ends the tick's publication: slots freed during the tick become reusable, policies
// no entry uses are dropped, and development builds check the derived state.
func (w *World) settleSync() {
	p := w.sync
	if !p.enabled() {
		return
	}
	p.roster.release()
	clear(p.retired)
	p.sweep()
	p.check()
}

// syncWire is a sync.Frame whose records were encoded once, when their entries were published.
// Its fields match sync.Frame in name and order, so it encodes to the same bytes.
type syncWire struct {
	Version     int
	Fingerprint string
	Stream      string
	Sequence    uint64
	Baseline    bool
	Records     []msgpack.RawMessage
	Removed     []syncstate.ID
	Access      []syncstate.Grant
}

func (syncWire) Name() string { return syncstate.EventName }

// sendSync numbers one frame on the reader's stream and queues it for the reader's writer.
func (w *World) sendSync(r *syncReader, baseline bool, d syncDelta) {
	r.sequence++
	if r.subscriber == nil || r.subscriber.ctx.Err() != nil {
		return
	}
	p := w.sync
	if p.fingerprint == "" {
		fingerprint, err := w.SyncContract().Fingerprint()
		if err != nil {
			panic(err)
		}
		p.fingerprint = fingerprint
	}
	data, err := schema.Serialize(syncWire{
		Version:     syncstate.ProtocolVersion,
		Fingerprint: p.fingerprint,
		Stream:      r.stream,
		Sequence:    r.sequence,
		Baseline:    baseline,
		Records:     d.records,
		Removed:     d.removed,
		Access:      d.access,
	})
	if err != nil {
		panic(err)
	}
	evt := &iscv1.Event{Name: syncstate.EventName, Payload: data}
	// A full queue ends the stream as lagged; the next reconcile closes its slot.
	_ = r.subscriber.enqueue(&cardinalv1.StartEventStreamResponse{Address: w.address, Event: evt}, proto.Size(evt))
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
