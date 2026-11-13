package cardinal

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/micro"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
	"github.com/vmihailenco/msgpack/v5"
)

// This test drives a real World with randomized ticks and checks, after every tick, that what
// each reader holds equals its slice of the world. The expected values come from an independent
// reading of the world through the public entity API (truth), which shares no code with the
// publisher. Each reader's holdings are rebuilt from the frames it received by a client that
// applies the compact layout's client rules (stream, gap, revision, removals, records, patches,
// grants); a patch merges into the held record. What each reader holds must also equal, field for
// field, the record a baseline would send it now. Every frame's Times add up to the server time
// of its tick, and a reader open across a tick is told something, an empty delta if nothing else,
// whenever its last frame is a heartbeat old: at every step, since steps are a second apart.
//
// Raise the seed count with SYNC_PROPERTY_SEEDS=n. A failure names its seed and tick.

type propValue struct {
	syncstate.State

	V int
}

func (propValue) Name() string { return "prop_value" }

// propTag is a second synced component, so a patch can carry one component and keep another.
type propTag struct {
	syncstate.State

	T int
}

func (propTag) Name() string { return "prop_tag" }

type propSynced struct {
	syncstate.Sync

	Value propValue
}

type propPlain struct{ Value propValue } // never synced

type propGroupRefs struct{ Group syncstate.Group }

type propPoke struct {
	syncstate.Target

	N int
}

func (propPoke) Name() string { return "prop_poke" }

type propShardOnly struct{ N int }

func (propShardOnly) Name() string { return "prop_shard_only" }

type propSpawn struct {
	syncstate.Event

	Entity uint32
	N      int
}

func (propSpawn) Name() string         { return "prop_spawn" }
func (e propSpawn) SyncEntity() uint32 { return e.Entity }

type propHit struct {
	syncstate.Event
	syncstate.Occurrence

	Entity uint32
	N      int
}

func (propHit) Name() string         { return "prop_hit" }
func (e propHit) SyncEntity() uint32 { return e.Entity }

var (
	propPlayers = []string{"alice", "bob", "carol", "dave"}
	propGroups  = []string{"red", "blue", "party"}
)

const propMaxEntities = 6

// propDropped is the verdict on a repeat of an admitted command: no handler, no rejection.
const propDropped syncstate.Reason = "dropped"

// propJudged is a command with an ID, and the verdict it got.
type propJudged struct {
	cmd     propPoke
	verdict syncstate.Reason
	at      int64
}

type propCommandSeen struct {
	sender, name string
	target       EntityID
}

type propHandler struct{ seen *[]propCommandSeen }

// propOps runs the tick's random operations as a system, after Cardinal judged the commands.
type propOps struct{ sim **propSim }

func (o propOps) Run(*World) {
	s := *o.sim
	for range 4 + s.rng.IntN(10) {
		s.op()
		if s.rng.IntN(3) == 0 {
			s.checkAuthorityOf(true) // mid-tick: after writes, before the tick's final capture
		}
	}
	s.checkAuthorityOf(true)
}

func (h propHandler) Run(w *World) {
	for cmd := range w.Commands[propPoke]() {
		*h.seen = append(*h.seen, propCommandSeen{cmd.Sender(), propPoke{}.Name(), cmd.Target().ID()})
	}
	for cmd := range w.Commands[propShardOnly]() {
		*h.seen = append(*h.seen, propCommandSeen{cmd.Sender(), propShardOnly{}.Name(), 0})
	}
}

// propHeld is one entity as a client holds it, decoded.
type propHeld struct {
	id       syncstate.ID
	access   syncstate.Access
	value    int
	hasValue bool
	tag      int
	hasTag   bool
}

// propClient is a client connection: the sync client, and the events it received.
type propClient struct {
	*syncClient

	fresh    bool         // a new connection: nothing may arrive before its baseline
	events   map[int]bool // event N received
	rejected map[string]syncstate.CommandRejected
}

type propReader struct {
	name       string
	sub        *streamSubscriber // nil while disconnected
	client     *propClient
	subscribed bool // to world_sync
	admitted   bool
	open       *streamSubscriber // the stream sync was open on at the end of the last tick
	frames     int               // sync frames its client applied this tick
	heartbeats int               // of which empty deltas
}

// propEvent is an emitted event and who must receive it.
type propEvent struct {
	n          int
	id         syncstate.ID
	occurrence bool
	recipient  string // "" for Broadcast
	ungated    bool
	expect     map[string]syncstate.Access // the truth it is judged by; nil: nobody
}

type propSim struct {
	t         *testing.T
	seed      uint64
	tick      int
	rng       *rand.Rand
	w         *TestWorld
	readers   []*propReader
	seen      []propCommandSeen
	rejects   map[string]syncstate.CommandRejected // recorded command_rejected sends: recipient/name -> payload
	verdicts  map[string]syncstate.Reason          // expected: sender/name -> reason ("" accepted)
	targets   map[string]EntityID
	ids       map[string]uint64       // sender/name -> the CommandID sent
	sent      map[string]propPoke     // sender/prop_poke -> the command sent
	repeats   map[string]bool         // sender/name sent an ID the engine remembers
	judgedAt  int64                   // the height, so the server time in seconds, this tick judges at
	nextID    uint64                  // the last CommandID handed out
	judged    map[string][]propJudged // per player, the IDs judged since the last reset
	debug     map[string]string
	events    []*propEvent
	lastTruth map[syncstate.ID]map[string]syncstate.Access // the lists each published lifetime was last captured with
	ended     map[syncstate.ID]map[string]syncstate.Access // the same, for lifetimes that ended this tick
	destroyed []syncstate.ID                               // lifetimes destroyed this tick
	nextN     int
	err       error
	stats     map[string]int
}

func (s *propSim) failf(format string, args ...any) {
	if s.err == nil {
		s.err = fmt.Errorf("seed %d tick %d: %s", s.seed, s.tick, fmt.Sprintf(format, args...))
	}
}

// -------------------------------------------------------------------------------------------------
// Truth: an independent reading of the world.
// -------------------------------------------------------------------------------------------------

func (s *propSim) synced(n EntityID) bool {
	e := s.w.Entity(n)
	return e.Alive() && e.Has[syncstate.Sync]()
}

func (s *propSim) groupMembers() map[string][][]string {
	out := map[string][][]string{}
	for e := range s.w.Contains[propGroupRefs]().Iter() {
		g := e.Get[syncstate.Group]()
		out[g.GroupName] = append(out[g.GroupName], slices.Collect(g.Players.Values()))
	}
	return out
}

// truth is one player's access to one entity from its lists alone; admission is separate.
func (s *propSim) truth(player string, n EntityID, groups map[string][][]string) syncstate.Access {
	if !s.synced(n) {
		return syncstate.None
	}
	in := func(players, names immutable.Slice[string]) bool {
		if immutable.Contains(players, player) {
			return true
		}
		for name := range names.Values() {
			if m := groups[name]; len(m) == 1 && slices.Contains(m[0], player) {
				return true
			}
		}
		return false
	}
	e := s.w.Entity(n)
	if e.Has[syncstate.Authorities]() {
		if a := e.Get[syncstate.Authorities](); in(a.Players, a.Groups) {
			return syncstate.Authority
		}
	}
	if !e.Has[syncstate.Viewers]() {
		return syncstate.View
	}
	if v := e.Get[syncstate.Viewers](); in(v.Players, v.Groups) {
		return syncstate.View
	}
	return syncstate.None
}

func (s *propSim) truths(n EntityID) map[string]syncstate.Access {
	groups := s.groupMembers()
	out := map[string]syncstate.Access{}
	for _, p := range propPlayers {
		out[p] = s.truth(p, n, groups)
	}
	return out
}

// captured follows a publication point: published lifetimes still synced take their lists now;
// the others end, and keep their last lists until the end of the tick. final also publishes the
// lifetimes that are new this tick.
func (s *propSim) captured(final bool) {
	for id, last := range s.lastTruth {
		n := EntityID(id.Entity)
		if !s.synced(n) || s.w.Entity(n).SyncID() != id {
			delete(s.lastTruth, id)
			s.ended[id] = last
		}
	}
	for _, n := range s.w.World.world.LiveEntityIDs() {
		id := s.w.Entity(n).SyncID()
		if _, published := s.lastTruth[id]; s.synced(n) && (published || final) {
			s.lastTruth[id] = s.truths(n)
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Readers and the client model
// -------------------------------------------------------------------------------------------------

func (s *propSim) connect(r *propReader) {
	ctx, cancel := context.WithCancelCause(context.Background())
	sub := &streamSubscriber{
		ctx:        ctx,
		cancel:     cancel,
		events:     map[string]struct{}{},
		queueReady: make(chan struct{}, 1),
	}
	events := []string{"prop_spawn", "prop_hit", "command_rejected"}
	if r.subscribed {
		events = append(events, syncstate.EventName)
	}
	addSubscriptions(sub, []*cardinalv1.EventSubscription{{Events: events}})
	s.w.service.addSubscriber(r.name, sub)
	r.sub, r.client = sub, s.newPropClient()
	r.client.fresh = true
}

func (s *propSim) disconnect(r *propReader) {
	if r.sub != nil {
		s.w.service.removeSubscriber(r.name, r.sub)
		r.sub.cancel(nil)
	}
	r.sub, r.client = nil, s.newPropClient()
}

func (s *propSim) setSubscribed(r *propReader, on bool) {
	r.subscribed = on
	if r.sub == nil {
		return
	}
	subscriptions := []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}}
	s.w.service.mu.Lock()
	if on {
		addSubscriptions(r.sub, subscriptions)
	} else {
		removeSubscriptions(r.sub, subscriptions)
	}
	s.w.service.mu.Unlock()
	if !on { // the client drops its view; deltas already in flight on the old stream are ignored
		r.client.fresh, r.client.ready = false, false
		r.client.clear()
	}
}

func (s *propSim) newPropClient() *propClient {
	return &propClient{
		syncClient: newSyncClient(s.w.SyncContract()),
		events:     map[int]bool{},
		rejected:   map[string]syncstate.CommandRejected{},
	}
}

// drain hands everything queued for a reader's connection to its client, in order.
func (s *propSim) drain(r *propReader) {
	if r.sub == nil {
		return
	}
	queued := r.sub.queue
	r.sub.queue, r.sub.queuedBytes = nil, 0
	for _, q := range queued {
		evt := q.response.GetEvent()
		switch evt.GetName() {
		case syncstate.EventName:
			s.receive(r, evt.GetPayload())
		case "prop_spawn", "prop_hit":
			var payload propSpawn
			if err := msgpack.Unmarshal(evt.GetPayload(), &payload); err != nil {
				s.failf("%s: decode event: %v", r.name, err)
			}
			r.client.events[payload.N] = true
		case "command_rejected":
			var payload syncstate.CommandRejected
			if err := msgpack.Unmarshal(evt.GetPayload(), &payload); err != nil {
				s.failf("%s: decode rejection: %v", r.name, err)
			}
			r.client.rejected[payload.Command] = payload
		}
	}
}

// receive hands one frame to the reader's client, and checks what the engine promises beyond
// the client rules.
func (s *propSim) receive(r *propReader, data []byte) {
	c := r.client
	var f syncstate.Frame
	if err := msgpack.Unmarshal(data, &f); err != nil {
		s.failf("%s: decode frame: %v", r.name, err)
		return
	}
	// Encode once: the wire bytes are exactly what encoding the frame's structs produces.
	if again := encodeSync(f); !bytes.Equal(again, data) {
		s.failf("%s: frame bytes differ from the struct encoding", r.name)
	}
	switch {
	case f.Open != nil:
		s.stats["baselines"]++
	case !c.ready && !c.fresh:
		s.stats["deltas ignored after resubscribing"]++
		return
	case !c.ready || f.Stream != c.stream:
		s.failf("%s: delta on stream %d before its baseline (holding %d, ready %v)",
			r.name, f.Stream, c.stream, c.ready)
		return
	case emptyDelta(f):
		s.stats["empty deltas (heartbeats)"]++
		r.heartbeats++
	}
	// Every revision of a lifetime a reader holds reaches it, so a patch, or a record over a held
	// lifetime, is exactly one revision on.
	removed := map[syncstate.Ref]bool{}
	for _, ref := range f.Removed {
		removed[ref] = true
	}
	for _, pa := range f.Patches {
		h := c.held[pa.Ref.Entity]
		if h.Ref == pa.Ref && pa.Revision != h.Revision+1 {
			s.failf("%s: patch %v to revision %d at %d", r.name, pa.Ref, pa.Revision, h.Revision)
		}
		for _, v := range h.Components {
			if !slices.Contains(pa.Dropped, v.Component) && !slices.Contains(componentsOf(pa.Changed), v.Component) {
				s.stats["patches that keep a held component"]++
				break
			}
		}
	}
	for _, rec := range f.Records {
		if h, ok := c.held[rec.Ref.Entity]; f.Open == nil && ok && h.Ref == rec.Ref && !removed[rec.Ref] {
			if rec.Revision != h.Revision+1 || rec.Presence == h.Presence {
				s.failf("%s: record %v at revision %d presence %d over revision %d presence %d",
					r.name, rec.Ref, rec.Revision, rec.Presence, h.Revision, h.Presence)
			}
			s.stats["records over a held lifetime (new entity presence)"]++
		}
	}
	before := c.time
	if _, err := c.apply(f); err != nil {
		s.failf("%s: %v", r.name, err)
	}
	r.frames++
	s.checkTime(r, f, before)
	s.checkPromise(r, f)
	s.tally(c, f)
}

// tally counts what a frame the client applied exercised.
func (s *propSim) tally(c *propClient, f syncstate.Frame) {
	if f.Open == nil {
		s.stats["deltas"]++
		if len(f.Records)+len(f.Patches)+len(f.Removed) == 0 && len(f.Access) > 0 {
			s.stats["access-only deltas"]++
		}
	}
	for _, ref := range f.Removed {
		if n := EntityID(ref.Entity); s.w.Entity(n).Alive() && s.w.Entity(n).SyncID() == c.id(ref) {
			s.stats["removals of a live entity (lost view)"]++
		}
	}
	for _, pa := range f.Patches {
		s.stats["patches"]++
		s.stats["patched components changed"] += len(pa.Changed)
		s.stats["patched components dropped"] += len(pa.Dropped)
	}
	s.stats["records"] += len(f.Records)
	s.stats["grants"] += len(f.Access)
}

// decode reads what a client holds of one entity through the contract.
func (s *propSim) decode(r *propReader, rec syncstate.Record) propHeld {
	h := propHeld{id: r.client.id(rec.Ref), access: r.client.access[rec.Ref.Entity]}
	contract := s.w.SyncContract()
	for _, v := range rec.Components {
		if v.Presence == 0 {
			s.failf("%s: component %d of %v has presence 0", r.name, v.Component, rec.Ref)
		}
		switch name := contract.Components[v.Component].Name; name {
		case propValue{}.Name():
			var value propValue
			if err := msgpack.Unmarshal(v.Data, &value); err != nil {
				s.failf("%s: decode value: %v", r.name, err)
			}
			h.value, h.hasValue = value.V, true
		case propTag{}.Name():
			var tag propTag
			if err := msgpack.Unmarshal(v.Data, &tag); err != nil {
				s.failf("%s: decode tag: %v", r.name, err)
			}
			h.tag, h.hasTag = tag.T, true
		default:
			s.failf("%s: unexpected component %q", r.name, name)
		}
	}
	return h
}

// -------------------------------------------------------------------------------------------------
// Operations
// -------------------------------------------------------------------------------------------------

func (s *propSim) names(from []string) immutable.Slice[string] {
	var out []string
	for _, v := range from {
		if s.rng.IntN(3) == 0 {
			out = append(out, v)
		}
	}
	return immutable.SliceOf(out...)
}

func (s *propSim) setLists(e Entity) {
	switch s.rng.IntN(4) {
	case 0:
		e.Remove[syncstate.Viewers]()
	default:
		e.Set(syncstate.Viewers{Players: s.names(propPlayers), Groups: s.names(propGroups)})
	}
}

func (s *propSim) setAuthorities(e Entity) {
	switch s.rng.IntN(4) {
	case 0:
		e.Remove[syncstate.Authorities]()
	default:
		e.Set(syncstate.Authorities{Players: s.names(propPlayers), Groups: s.names(propGroups)})
	}
}

// entities returns the live entities that are not group holders.
func (s *propSim) entities() []EntityID {
	var out []EntityID
	for _, n := range s.w.World.world.LiveEntityIDs() {
		if !s.w.Entity(n).Has[syncstate.Group]() {
			out = append(out, n)
		}
	}
	return out
}

func (s *propSim) pick(ids []EntityID) (EntityID, bool) {
	if len(ids) == 0 {
		return 0, false
	}
	return ids[s.rng.IntN(len(ids))], true
}

func (s *propSim) create() {
	if len(s.entities()) >= propMaxEntities {
		return
	}
	var e Entity
	if s.rng.IntN(5) == 0 {
		e = s.w.Create[propPlain]()
	} else {
		e = s.w.Create[propSynced]()
	}
	e.Set(propValue{V: s.rng.IntN(3)})
	if s.rng.IntN(2) == 0 {
		e.Set(propTag{T: s.rng.IntN(3)})
	}
	if s.rng.IntN(2) == 0 {
		s.setLists(e)
	}
	if s.rng.IntN(2) == 0 {
		s.setAuthorities(e)
	}
}

// emit sends an event about id, judged by the truth at this point in the tick.
func (s *propSim) emit(id syncstate.ID) {
	s.nextN++
	ev := &propEvent{n: s.nextN, id: id, occurrence: s.rng.IntN(2) == 0}
	if s.rng.IntN(4) == 0 {
		ev.recipient = propPlayers[s.rng.IntN(len(propPlayers))]
	}
	n := EntityID(id.Entity)
	alive := s.w.Entity(n).Alive() && s.w.Entity(n).SyncID() == id
	last, published := s.lastTruth[id]
	if ended, ok := s.ended[id]; ok {
		last, published = ended, true
	}
	switch {
	case alive && s.synced(n):
		ev.expect = s.truths(n)
	case published:
		ev.expect = last
	case alive:
		ev.ungated = true // never synced as this lifetime
	default:
	}
	stamp := syncstate.For(id)
	var payload any
	if ev.occurrence {
		payload = propHit{Event: stamp, Entity: id.Entity, N: ev.n}
	} else {
		payload = propSpawn{Event: stamp, Entity: id.Entity, N: ev.n}
	}
	switch p := payload.(type) {
	case propHit:
		if ev.recipient == "" {
			s.w.Broadcast(p)
		} else {
			s.w.SendTo(ev.recipient, p)
		}
	case propSpawn:
		if ev.recipient == "" {
			s.w.Broadcast(p)
		} else {
			s.w.SendTo(ev.recipient, p)
		}
	}
	s.captured(false)
	s.events = append(s.events, ev)
}

func (s *propSim) groupOp() {
	var holders []EntityID
	for e := range s.w.Contains[propGroupRefs]().Iter() {
		holders = append(holders, e.ID())
	}
	switch k := s.rng.IntN(4); {
	case k == 0 || len(holders) == 0:
		e := s.w.Create[propGroupRefs]()
		e.Set(syncstate.Group{GroupName: propGroups[s.rng.IntN(len(propGroups))], Players: s.names(propPlayers)})
	case k == 1:
		e := s.w.Entity(holders[s.rng.IntN(len(holders))])
		g := e.Get[syncstate.Group]()
		g.Players = s.names(propPlayers)
		e.Set(g)
	case k == 2:
		e := s.w.Entity(holders[s.rng.IntN(len(holders))])
		g := e.Get[syncstate.Group]()
		g.GroupName = propGroups[s.rng.IntN(len(propGroups))]
		e.Set(g)
	default:
		s.w.Entity(holders[s.rng.IntN(len(holders))]).Destroy()
	}
}

func (s *propSim) readerOp() {
	r := s.readers[s.rng.IntN(len(s.readers))]
	switch s.rng.IntN(5) {
	case 0, 1:
		r.admitted = !r.admitted
	case 2:
		if r.sub == nil {
			s.connect(r)
		} else {
			s.disconnect(r)
		}
	case 3:
		s.connect(r) // reconnect: the new stream replaces the old
	default:
		s.setSubscribed(r, !r.subscribed)
	}
}

// toggle removes one synced component from an entity, or adds it back.
func (s *propSim) toggle(e Entity) {
	if s.rng.IntN(2) == 0 {
		if e.Has[propValue]() {
			e.Remove[propValue]()
		} else {
			e.Set(propValue{V: 1})
		}
		return
	}
	if e.Has[propTag]() {
		e.Remove[propTag]()
	} else {
		e.Set(propTag{T: 1})
	}
}

func (s *propSim) op() {
	ids := s.entities()
	n, ok := s.pick(ids)
	switch s.rng.IntN(16) {
	case 0, 1:
		s.create()
	case 2:
		if ok {
			id := s.w.Entity(n).SyncID()
			s.w.Entity(n).Destroy()
			s.destroyed = append(s.destroyed, id)
			if s.rng.IntN(2) == 0 {
				s.emit(id) // a despawn sent after Destroy, with the identity captured first
			}
			if s.rng.IntN(3) == 0 {
				s.create() // the numeric ID is reused within the tick
			}
		}
	case 3, 4:
		if ok && s.rng.IntN(2) == 0 {
			s.w.Entity(n).Set(propValue{V: s.rng.IntN(3)})
		} else if ok {
			s.w.Entity(n).Set(propTag{T: s.rng.IntN(3)})
		}
	case 5:
		if ok {
			s.toggle(s.w.Entity(n))
		}
	case 6:
		if ok {
			s.setLists(s.w.Entity(n))
		}
	case 7:
		if ok {
			s.setAuthorities(s.w.Entity(n))
		}
	case 8:
		if ok {
			if s.w.Entity(n).Has[syncstate.Sync]() {
				s.w.Entity(n).Remove[syncstate.Sync]()
			} else {
				s.w.Entity(n).Set(syncstate.Sync{})
			}
		}
	case 9, 10:
		s.groupOp()
	case 11, 12:
		if ok {
			id := s.w.Entity(n).SyncID()
			switch {
			case s.rng.IntN(5) == 0:
				id.Incarnation += uint64(s.rng.IntN(3)) + 1 // a lifetime that is not this one
			case s.rng.IntN(3) == 0 && len(s.destroyed) > 0: // destroyed earlier in the tick
				id = s.destroyed[s.rng.IntN(len(s.destroyed))]
			}
			s.emit(id)
		}
	case 13, 14:
		s.readerOp()
	default:
		if s.rng.IntN(8) == 0 { // snapshot restore: every lifetime is replaced
			state, err := s.w.World.world.ToProto()
			if err != nil {
				panic(err)
			}
			if err := s.w.World.world.FromProto(state); err != nil {
				panic(err)
			}
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Commands
// -------------------------------------------------------------------------------------------------

// sendCommands queues this tick's commands and records the verdict each must get. The target's
// truth is the world now, which is what the last tick published; admission is judged at drain.
func (s *propSim) sendCommands() {
	s.verdicts, s.targets, s.debug = map[string]syncstate.Reason{}, map[string]EntityID{}, map[string]string{}
	s.ids, s.sent, s.repeats = map[string]uint64{}, map[string]propPoke{}, map[string]bool{}
	s.judgedAt = int64(s.w.TickHeight())
	ids := s.entities()
	for _, player := range propPlayers {
		if s.rng.IntN(3) == 0 {
			continue
		}
		key := player + "/prop_poke"
		if prev, ok := s.recent(player); ok && s.rng.IntN(4) == 0 {
			// A retry inside the memory gets the first verdict; an admitted one is dropped.
			s.verdicts[key], s.repeats[key] = prev.verdict, true
			if prev.verdict == "" {
				s.verdicts[key] = propDropped
			}
			s.targets[key], s.ids[key] = EntityID(prev.cmd.TargetEntity), prev.cmd.CommandID
			s.w.Command(player, prev.cmd)
			continue
		}
		var target syncstate.ID
		if n, ok := s.pick(ids); ok {
			target = s.w.Entity(n).SyncID()
			switch s.rng.IntN(6) {
			case 0:
				target.Incarnation++
			case 1:
				target.Session = "elsewhere"
			}
		} else {
			target = syncstate.ID{Entity: uint32(s.rng.IntN(propMaxEntities))}
		}
		n := EntityID(target.Entity)
		want := syncstate.StaleTarget
		if s.synced(n) && s.w.Entity(n).SyncID() == target {
			switch s.truth(player, n, s.groupMembers()) {
			case syncstate.Authority:
				want = ""
			case syncstate.View:
				want = syncstate.NoAuthority
			case syncstate.None:
			}
		}
		s.verdicts[key], s.targets[key] = want, n
		if en := s.w.sync.entry(n); en != nil {
			s.debug[key] = fmt.Sprintf(
				"target %v truths %v entry %v key %q access %v groups %v plists %v stale %v",
				target,
				s.truths(n),
				en.id,
				en.policy.key,
				s.w.sync.access(en.policy, player),
				s.groupMembers(),
				en.policy.lists,
				en.policy.stale,
			)
		}
		cmd := propPoke{Target: syncstate.TargetOf(target), N: s.rng.IntN(100)}
		if s.rng.IntN(5) != 0 { // else 0: no ID
			s.nextID++
			cmd.CommandID = s.nextID
		}
		s.ids[key], s.sent[key] = cmd.CommandID, cmd
		s.w.Command(player, cmd)
		if s.rng.IntN(4) == 0 {
			s.verdicts[player+"/prop_shard_only"] = syncstate.ShardOnly
			s.w.Command(player, propShardOnly{})
		}
	}
	if s.rng.IntN(3) == 0 { // a shard may not send a player-only command
		shard := micro.GetAddress("other", "shard")
		payload, err := schema.Serialize(propPoke{})
		if err != nil {
			panic(err)
		}
		err = s.w.commands.Enqueue(
			context.Background(),
			&iscv1.Command{Name: "prop_poke", Address: s.w.address, Payload: payload},
			command.ShardSender(shard),
		)
		if err != nil {
			panic(err)
		}
		s.verdicts[command.ShardSender(shard).ID()+"/prop_poke"] = syncstate.PlayerOnly
	}
}

// recent picks one of the player's IDs the engine still remembers: judged since the last reset,
// less than 30 s of server time before this tick. Every player sends at most one new ID a tick,
// so the 256 cap is never reached here.
func (s *propSim) recent(player string) (propJudged, bool) {
	var inside []propJudged
	for _, j := range s.judged[player] {
		if s.judgedAt-j.at < 30 {
			inside = append(inside, j)
		}
	}
	if len(inside) == 0 {
		return propJudged{}, false
	}
	return inside[s.rng.IntN(len(inside))], true
}

// checkCommands compares the verdicts with the truth. Admission is judged when the tick drains
// the commands, before this tick's operations.
func (s *propSim) checkCommands(admitted map[string]bool) {
	got := map[string]syncstate.Reason{}
	for key, r := range s.rejects {
		got[key] = r.Reason
		if r.CommandID != s.ids[key] {
			s.failf("command %s: rejection names CommandID %d, want %d", key, r.CommandID, s.ids[key])
		}
	}
	for _, seen := range s.seen {
		key := seen.sender + "/" + seen.name
		if _, dup := got[key]; dup {
			s.failf("command %s both admitted and rejected, or admitted twice", key)
		}
		got[key] = ""
		if seen.name == "prop_poke" && seen.target != s.targets[key] {
			s.failf("command %s: Target() is %d, want %d", key, seen.target, s.targets[key])
		}
	}
	for key, want := range s.verdicts {
		player, name, _ := strings.Cut(key, "/")
		if (want == "" || want == syncstate.NoAuthority) && !s.repeats[key] {
			if !admitted[player] {
				want = syncstate.StaleTarget
			}
		}
		if name == "prop_poke" && !s.repeats[key] && s.ids[key] != 0 {
			s.judged[player] = append(s.judged[player], propJudged{s.sent[key], want, s.judgedAt})
		}
		reason, known := got[key]
		switch want { //nolint:exhaustive // the rest leave a verdict
		case syncstate.PlayerOnly: // a shard has no stream: the rejection is only logged
			if known {
				s.failf("command %s from a shard reached a handler", key)
			}
			continue
		case propDropped:
			if known {
				s.failf("repeat %s of an admitted command got %q (\"\": it ran again)", key, reason)
			}
			s.stats["commands: repeat dropped"]++
			continue
		}
		if s.repeats[key] {
			s.stats["commands: repeat rejected again"]++
		}
		if !known || reason != want {
			s.failf("command %s: got %q (known %v), want %q; %s", key, reason, known, want, s.debug[key])
		}
		label := string(want)
		if want == "" {
			label = "accepted"
		}
		s.stats["commands: "+label]++
	}
	if len(got) != len(s.verdicts)-countUnobserved(s.verdicts) {
		s.failf("commands: %d verdicts observed, %d expected", len(got), len(s.verdicts))
	}
}

// countUnobserved counts the verdicts that leave neither a handler run nor a rejection.
func countUnobserved(v map[string]syncstate.Reason) int {
	n := 0
	for _, r := range v {
		if r == syncstate.PlayerOnly || r == propDropped {
			n++
		}
	}
	return n
}

// -------------------------------------------------------------------------------------------------
// The tick and its checks
// -------------------------------------------------------------------------------------------------

func (s *propSim) step() {
	s.seen, s.rejects, s.events = nil, map[string]syncstate.CommandRejected{}, nil
	s.destroyed, s.ended = nil, map[syncstate.ID]map[string]syncstate.Access{}
	reset := s.rng.IntN(60) == 0
	if reset { // only between ticks, as the debug service does; it drops queued commands
		s.w.World.reset()
		s.verdicts = map[string]syncstate.Reason{}
		s.judged = map[string][]propJudged{} // the test clock rewinds; IDs from before are not reused
	} else {
		s.sendCommands()
	}
	admitted := map[string]bool{}
	for _, r := range s.readers {
		admitted[r.name] = r.admitted
	}
	s.w.Tick() // judges the commands, then runs propOps

	s.captured(true)

	// An event about a lifetime that is published at the end of the tick is judged by its lists then.
	for _, ev := range s.events {
		if n := EntityID(ev.id.Entity); s.synced(n) && s.w.Entity(n).SyncID() == ev.id {
			ev.expect, ev.ungated = s.truths(n), false
		}
	}
	now := uint64(s.w.Timestamp().UnixMilli())
	for _, r := range s.readers {
		wasOpen, before, promise := r.open != nil && r.open == r.sub, r.client.time, r.client.heartbeat
		r.frames, r.heartbeats = 0, 0
		s.drain(r)
		// A stream whose baseline promised nothing is never due.
		s.checkHeartbeat(r, wasOpen && r.sub != nil && r.subscribed && r.admitted, promise > 0 && now >= before+promise)
		// A connected sender hears about each of its rejected commands on its stream.
		for key, sent := range s.rejects {
			if sender, name, _ := strings.Cut(
				key,
				"/",
			); sender == r.name && r.sub != nil &&
				r.client.rejected[name] != sent {
				s.failf("%s did not receive its rejection of %s (%v)", r.name, name, sent)
			}
		}
		r.client.rejected = map[string]syncstate.CommandRejected{}
	}
	if !reset {
		s.checkCommands(admitted)
	}
	s.checkEvents()
	s.checkReplicas()
}

// checkTime checks that the Times a client added up, before was the sum before f, are f's tick
// time: the tick's own on a baseline, else the latest tick time since the baseline, since a tick
// whose time went back, as after a reset, adds 0.
func (s *propSim) checkTime(r *propReader, f syncstate.Frame, before uint64) {
	now := uint64(s.w.Timestamp().UnixMilli())
	want := max(before, now)
	if f.Open != nil {
		want = now
	}
	if r.client.time != want {
		s.failf("%s: frame %d's Times add up to %d, its tick is at %d", r.name, f.Sequence, r.client.time, want)
	}
}

// checkPromise checks a baseline's heartbeat: the engine's to a reader it seats, none to a reader
// whose view it clears because its admission was revoked.
func (s *propSim) checkPromise(r *propReader, f syncstate.Frame) {
	if f.Open == nil {
		return
	}
	want := uint64(0)
	if r.admitted {
		want = heartbeatMillis
	}
	if f.Open.Heartbeat != want {
		s.failf("%s: baseline promises a heartbeat of %d ms, admitted %v", r.name, f.Open.Heartbeat, r.admitted)
	}
	s.stats[fmt.Sprintf("baselines promising %d ms", f.Open.Heartbeat)]++
}

// checkHeartbeat checks the heartbeat rule for one reader's tick: a reader open across the tick
// whose last frame was a heartbeat old is told something, and an empty delta is sent only to such
// a reader, as the tick's one frame.
func (s *propSim) checkHeartbeat(r *propReader, stayedOpen, due bool) {
	switch {
	case r.heartbeats > 0 && (!due || r.frames != 1):
		s.failf("%s: %d empty deltas among %d frames, due %v", r.name, r.heartbeats, r.frames, due)
	case stayedOpen && due && r.frames == 0:
		s.failf("%s: no frame for a heartbeat", r.name)
	case stayedOpen && !due:
		s.stats["open readers not yet due a heartbeat (time went back)"]++
	}
}

func (s *propSim) checkEvents() {
	for _, ev := range s.events {
		for _, r := range s.readers {
			want := r.sub != nil && (ev.recipient == "" || ev.recipient == r.name)
			switch {
			case ev.occurrence:
				want = want && r.open != nil && r.open == r.sub && (ev.ungated || ev.expect[r.name] >= syncstate.View)
			default:
				want = want && (ev.ungated || (r.admitted && ev.expect[r.name] >= syncstate.View))
			}
			got := r.sub != nil && r.client.events[ev.n]
			if got != want {
				s.failf("event %d about %v (occurrence %v, ungated %v, recipient %q) for %s: delivered %v, want %v "+
					"(open %v, connected %v, admitted %v, truth %v)", ev.n, ev.id, ev.occurrence, ev.ungated, ev.recipient,
					r.name, got, want, r.open != nil && r.open == r.sub, r.sub != nil, r.admitted, ev.expect)
			}
			if got {
				s.stats["events delivered"]++
			} else {
				s.stats["events withheld"]++
			}
			// Never told about a lifetime whose state it will not receive.
			n := EntityID(ev.id.Entity)
			if got && !ev.ungated && r.sub != nil && r.subscribed && r.admitted && s.synced(n) &&
				s.w.Entity(n).SyncID() == ev.id && !r.client.holds(ev.id) {
				s.failf("%s was told about %v but holds no state for it", r.name, ev.id)
			}
		}
	}
}

func (s *propSim) checkReplicas() {
	groups := s.groupMembers()
	for _, r := range s.readers {
		open := r.sub != nil && r.subscribed && r.admitted
		want := map[uint32]propHeld{}
		if open {
			for _, n := range s.w.World.world.LiveEntityIDs() {
				if a := s.truth(r.name, n, groups); a != syncstate.None {
					e := s.w.Entity(n)
					h := propHeld{id: e.SyncID(), access: a}
					if e.Has[propValue]() {
						h.value, h.hasValue = e.Get[propValue]().V, true
					}
					if e.Has[propTag]() {
						h.tag, h.hasTag = e.Get[propTag]().T, true
					}
					want[uint32(n)] = h
				}
			}
		}
		got := map[uint32]propHeld{}
		for k, rec := range r.client.held {
			got[k] = s.decode(r, rec)
			// What it holds, patches merged, is the record a baseline would send it now.
			var fresh syncstate.Record
			if e := s.w.sync.entry(EntityID(k)); e == nil || msgpack.Unmarshal(e.record, &fresh) != nil {
				s.failf("%s holds %v, which has no published record", r.name, rec.Ref)
			} else if !reflect.DeepEqual(normalized(rec), normalized(fresh)) {
				s.failf("%s holds %+v, a baseline would send %+v", r.name, rec, fresh)
			}
		}
		if !maps.Equal(got, want) {
			s.failf("%s (open %v) holds %v, its slice is %v", r.name, open, got, want)
		}
		if open && !r.client.ready {
			s.failf("%s is open but its client has no baseline", r.name)
		}
		r.open = nil
		if open {
			r.open = r.sub
		}
	}
	s.checkAuthorityOf(false)
}

// checkAuthorityOf compares AuthorityOf with the truth, read from the world as it is now. Mid-tick,
// it also counts the answers that the last publication would have got wrong.
func (s *propSim) checkAuthorityOf(midTick bool) {
	groups := s.groupMembers()
	for _, r := range s.readers {
		var want, got, published []EntityID
		for _, n := range s.w.World.world.LiveEntityIDs() {
			if r.admitted && s.truth(r.name, n, groups) == syncstate.Authority {
				want = append(want, n)
			}
			if e := s.w.sync.entry(n); r.admitted && e != nil && e.policy.access(r.name) == syncstate.Authority {
				published = append(published, n)
			}
		}
		for e := range s.w.AuthorityOf(r.name) {
			got = append(got, e.ID())
		}
		if !slices.Equal(got, want) {
			s.failf("AuthorityOf(%s) = %v, truth %v (mid-tick %v)", r.name, got, want, midTick)
		}
		if midTick {
			s.stats["AuthorityOf: mid-tick checks"]++
			if !slices.Equal(published, want) {
				s.stats["AuthorityOf: mid-tick checks where the last publication differs"]++
			}
		}
	}
}

// normalized makes an empty list and a nil list equal, as the layout says they are.
func normalized(r syncstate.Record) syncstate.Record {
	if len(r.Components) == 0 {
		r.Components = nil
	}
	return r
}

func runSyncProperty(t *testing.T, seed uint64, ticks int, stats map[string]int) error {
	s := &propSim{
		t:         t,
		seed:      seed,
		rng:       rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		lastTruth: map[syncstate.ID]map[string]syncstate.Access{},
		ended:     map[syncstate.ID]map[string]syncstate.Access{},
		judged:    map[string][]propJudged{},
		stats:     stats,
	}
	s.w = NewTestWorld(t, func(w *World) {
		w.RegisterComponent[propValue]()
		w.RegisterComponent[propTag]()
		w.EnableSync(func(player string) bool {
			for _, r := range s.readers {
				if r.name == player {
					return r.admitted
				}
			}
			return false
		})
		w.RegisterCommand[propPoke](syncstate.FromPlayers)
		w.RegisterCommand[propShardOnly](syncstate.FromShards)
		w.RegisterEvent[propSpawn]()
		w.RegisterEvent[propHit]()
		w.RegisterSystem(propHandler{&s.seen}, WithHook(PreUpdate))
		w.RegisterSystem(propOps{&s})
	})
	s.w.tel.Logger = zerolog.Nop()
	s.w.World.events.RegisterHandler(event.KindDefault, func(ctx context.Context, evt event.Event) error {
		if rejected, ok := evt.Payload.(syncstate.CommandRejected); ok {
			s.rejects[evt.Recipient+"/"+rejected.Command] = rejected
		}
		return s.w.service.publishDefaultEvent(ctx, evt)
	})
	for _, p := range propPlayers {
		r := &propReader{name: p, subscribed: true, admitted: s.rng.IntN(4) != 0}
		s.readers = append(s.readers, r)
		s.connect(r)
	}
	for s.tick = range ticks {
		s.step()
		s.stats["ticks"]++
		if s.err != nil {
			return s.err
		}
	}
	return nil
}

func TestSyncPropertyMatchesIndependentReading(t *testing.T) {
	seeds := 30
	if v := os.Getenv("SYNC_PROPERTY_SEEDS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			t.Fatal(err)
		}
		seeds = parsed
	}
	stats := map[string]int{}
	for seed := range uint64(seeds) {
		if err := runSyncProperty(t, seed, 150, stats); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(stats)) {
		t.Logf("%-45s %d", k, stats[k])
	}
}
