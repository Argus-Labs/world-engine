package cardinal

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
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
// applies the Unity client's checks (stream, gap, revision, removals, grants).
//
// Raise the seed count with SYNC_PROPERTY_SEEDS=n. A failure names its seed and tick.

type propValue struct {
	syncstate.State

	V int
}

func (propValue) Name() string { return "prop_value" }

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

// propHeld is one entity as a client holds it.
type propHeld struct {
	id       syncstate.ID
	revision uint64
	access   syncstate.Access
	value    int
	hasValue bool
}

// propClient is a client connection. It mirrors the checks Unity's SyncBridge makes.
type propClient struct {
	fresh    bool // a new connection: nothing may arrive before its baseline
	ready    bool
	stream   string
	sequence uint64
	held     map[uint32]propHeld
	events   map[int]bool // event N received
	rejected map[string]syncstate.Reason
}

type propReader struct {
	name       string
	sub        *streamSubscriber // nil while disconnected
	client     *propClient
	subscribed bool // to world_sync
	admitted   bool
	open       *streamSubscriber // the stream sync was open on at the end of the last tick
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
	rejects   map[string]syncstate.Reason // recorded command_rejected sends: recipient/name -> reason
	verdicts  map[string]syncstate.Reason // expected: sender/name -> reason ("" accepted)
	targets   map[string]EntityID
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
	r.sub, r.client = sub, newPropClient()
	r.client.fresh = true
}

func (s *propSim) disconnect(r *propReader) {
	if r.sub != nil {
		s.w.service.removeSubscriber(r.name, r.sub)
		r.sub.cancel(nil)
	}
	r.sub, r.client = nil, newPropClient()
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
		r.client.fresh, r.client.ready, r.client.held = false, false, map[uint32]propHeld{}
	}
}

func newPropClient() *propClient {
	return &propClient{held: map[uint32]propHeld{}, events: map[int]bool{}, rejected: map[string]syncstate.Reason{}}
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
			r.client.rejected[payload.Command] = payload.Reason
		}
	}
}

//nolint:gocyclo,cyclop // One place for every check the client makes, in the order it makes them.
func (s *propSim) receive(r *propReader, data []byte) {
	c := r.client
	var f syncstate.Frame
	if err := msgpack.Unmarshal(data, &f); err != nil {
		s.failf("%s: decode frame: %v", r.name, err)
		return
	}
	// Encode once: the wire bytes are exactly what marshalling the frame's structs produces.
	if again, err := schema.Serialize(f); err != nil || !bytes.Equal(again, data) {
		s.failf("%s: frame bytes differ from the struct encoding (%v)", r.name, err)
	}
	if f.Version != syncstate.ProtocolVersion || f.Fingerprint == "" || f.Stream == "" {
		s.failf("%s: bad header %d %q %q", r.name, f.Version, f.Fingerprint, f.Stream)
	}
	if f.Baseline {
		if len(f.Removed) != 0 {
			s.failf("%s: baseline with removals", r.name)
		}
		c.ready, c.stream, c.held = true, f.Stream, map[uint32]propHeld{}
		s.stats["baselines"]++
	} else {
		if !c.ready && !c.fresh {
			s.stats["deltas ignored after resubscribing"]++
			return
		}
		switch {
		case !c.ready || f.Stream != c.stream:
			s.failf(
				"%s: delta on stream %q before its baseline (holding %q, ready %v)",
				r.name,
				f.Stream,
				c.stream,
				c.ready,
			)
		case f.Sequence != c.sequence+1:
			s.failf("%s: sequence %d after %d", r.name, f.Sequence, c.sequence)
		case len(f.Records)+len(f.Removed)+len(f.Access) == 0:
			s.failf("%s: empty delta", r.name)
		}
		s.stats["deltas"]++
		if len(f.Records) == 0 && len(f.Removed) == 0 {
			s.stats["access-only deltas"]++
		}
	}
	c.sequence = f.Sequence
	for _, id := range f.Removed {
		if cur, ok := c.held[id.Entity]; !ok || cur.id != id {
			s.failf("%s: removed %v, which it does not hold", r.name, id)
		}
		delete(c.held, id.Entity)
		if n := EntityID(id.Entity); s.w.Entity(n).Alive() && s.w.Entity(n).SyncID() == id {
			s.stats["removals of a live entity (lost view)"]++
		}
	}
	for _, rec := range f.Records {
		cur, ok := c.held[rec.ID.Entity]
		if ok && cur.id != rec.ID {
			s.failf("%s: record %v over %v without a removal", r.name, rec.ID, cur.id)
		}
		if ok && rec.Revision <= cur.revision {
			s.failf("%s: revision %d after %d for %v", r.name, rec.Revision, cur.revision, rec.ID)
		}
		next := propHeld{id: rec.ID, revision: rec.Revision, access: cur.access}
		for _, v := range rec.Components {
			if v.Name != (propValue{}).Name() || v.Presence == 0 {
				s.failf("%s: unexpected component %q presence %d", r.name, v.Name, v.Presence)
				continue
			}
			var value propValue
			if err := msgpack.Unmarshal(v.Data, &value); err != nil {
				s.failf("%s: decode value: %v", r.name, err)
			}
			next.value, next.hasValue = value.V, true
		}
		c.held[rec.ID.Entity] = next
		s.stats["records"]++
	}
	for _, g := range f.Access {
		cur, ok := c.held[g.ID.Entity]
		if !ok || cur.id != g.ID {
			s.failf("%s: grant for %v, which it does not hold", r.name, g.ID)
		}
		cur.access = g.Access
		c.held[g.ID.Entity] = cur
		s.stats["grants"]++
	}
	for _, h := range c.held {
		if h.access == syncstate.None {
			s.failf("%s: holds %v without access", r.name, h.id)
		}
	}
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
		if ok {
			s.w.Entity(n).Set(propValue{V: s.rng.IntN(3)})
		}
	case 5:
		if ok {
			if s.w.Entity(n).Has[propValue]() {
				s.w.Entity(n).Remove[propValue]()
			} else {
				s.w.Entity(n).Set(propValue{V: 1})
			}
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
	ids := s.entities()
	for _, player := range propPlayers {
		if s.rng.IntN(3) == 0 {
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
		key := player + "/prop_poke"
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
		s.w.Command(player, propPoke{Target: syncstate.TargetOf(target), N: s.rng.IntN(100)})
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

// checkCommands compares the verdicts with the truth. Admission is judged when the tick drains
// the commands, before this tick's operations.
func (s *propSim) checkCommands(admitted map[string]bool) {
	got := maps.Clone(s.rejects)
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
		player, _, _ := strings.Cut(key, "/")
		if want == "" || want == syncstate.NoAuthority {
			if !admitted[player] {
				want = syncstate.StaleTarget
			}
		}
		reason, known := got[key]
		if want == syncstate.PlayerOnly { // a shard has no stream: the rejection is only logged
			if known {
				s.failf("command %s from a shard reached a handler", key)
			}
			continue
		}
		if !known || reason != want {
			s.failf("command %s: got %q (known %v), want %q; %s", key, reason, known, want, s.debug[key])
		}
		name := string(want)
		if want == "" {
			name = "accepted"
		}
		s.stats["commands: "+name]++
	}
	if len(got) != len(s.verdicts)-countPlayerOnly(s.verdicts) {
		s.failf("commands: %d verdicts observed, %d expected", len(got), len(s.verdicts))
	}
}

func countPlayerOnly(v map[string]syncstate.Reason) int {
	n := 0
	for _, r := range v {
		if r == syncstate.PlayerOnly {
			n++
		}
	}
	return n
}

// -------------------------------------------------------------------------------------------------
// The tick and its checks
// -------------------------------------------------------------------------------------------------

func (s *propSim) step() {
	s.seen, s.rejects, s.events = nil, map[string]syncstate.Reason{}, nil
	s.destroyed, s.ended = nil, map[syncstate.ID]map[string]syncstate.Access{}
	reset := s.rng.IntN(60) == 0
	if reset { // only between ticks, as the debug service does; it drops queued commands
		s.w.World.reset()
		s.verdicts = map[string]syncstate.Reason{}
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
	for _, r := range s.readers {
		s.drain(r)
		// A connected sender hears about each of its rejected commands on its stream.
		for key, reason := range s.rejects {
			if sender, name, _ := strings.Cut(
				key,
				"/",
			); sender == r.name && r.sub != nil &&
				r.client.rejected[name] != reason {
				s.failf("%s did not receive its rejection of %s (%q)", r.name, name, reason)
			}
		}
		r.client.rejected = map[string]syncstate.Reason{}
	}
	if !reset {
		s.checkCommands(admitted)
	}
	s.checkEvents()
	s.checkReplicas()
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
				s.w.Entity(n).SyncID() == ev.id && r.client.held[ev.id.Entity].id != ev.id {
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
					want[uint32(n)] = h
				}
			}
		}
		got := map[uint32]propHeld{}
		for k, h := range r.client.held {
			h.revision = 0
			got[k] = h
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

func runSyncProperty(t *testing.T, seed uint64, ticks int, stats map[string]int) error {
	s := &propSim{
		t:         t,
		seed:      seed,
		rng:       rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		lastTruth: map[syncstate.ID]map[string]syncstate.Access{},
		ended:     map[syncstate.ID]map[string]syncstate.Access{},
		stats:     stats,
	}
	s.w = NewTestWorld(t, func(w *World) {
		w.RegisterComponent[propValue]()
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
			s.rejects[evt.Recipient+"/"+rejected.Command] = rejected.Reason
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
