package cardinal

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

type syncHealth struct {
	syncstate.State

	HP    int
	MaxHP int
}

func (syncHealth) Name() string { return "health" }

type syncPrivate struct{ Secret string }

func (syncPrivate) Name() string { return "private" }

type syncAvatar struct {
	syncstate.Sync

	Health  syncHealth
	Private syncPrivate
}
type syncHurt struct {
	syncstate.Occurrence

	Amount int
}

func (syncHurt) Name() string { return "hurt" }

func newSyncFixture(t *testing.T, admit func(string) bool) (*TestWorld, *streamSubscriber) {
	w := NewTestWorld(t, func(w *World) {
		w.RegisterComponent[syncHealth]()
		w.RegisterComponent[syncPrivate]()
		w.EnableSync(admit)
		w.RegisterEvent[syncHurt]()
		w.RegisterEvent[syncReply]()
	})
	w.World.events.RegisterHandler(event.KindDefault, w.service.publishDefaultEvent)
	ctx, cancel := context.WithCancelCause(t.Context())
	sub := &streamSubscriber{
		ctx:        ctx,
		cancel:     cancel,
		events:     map[string]struct{}{syncstate.EventName: {}, "hurt": {}},
		queueReady: make(chan struct{}, 1),
	}
	w.service.addSubscriber("alice", sub)
	return w, sub
}

// syncClient applies one connection's frames by the client rules of the compact layout: it
// validates a whole frame before it changes anything, then applies removals, records, patches
// and grants, in that order. It shares no code with the publisher.
type syncClient struct {
	fingerprint string // the contract's; "" accepts any
	components  int    // how many components the contract lists
	ready       bool
	session     string
	stream      uint64
	sequence    uint64
	time        uint64                      // the stream's server time: its frames' Times added up
	heartbeat   uint64                      // the stream's promise, in ms; 0: none
	held        map[uint32]syncstate.Record // by entity, as a whole record would carry it
	access      map[uint32]syncstate.Access
}

func newSyncClient(contract syncstate.Contract) *syncClient {
	c := &syncClient{components: len(contract.Components)}
	fingerprint, err := contract.Fingerprint()
	if err != nil {
		panic(err)
	}
	c.fingerprint = fingerprint
	c.clear()
	return c
}

func (c *syncClient) clear() {
	c.held, c.access = map[uint32]syncstate.Record{}, map[uint32]syncstate.Access{}
}

// holds reports whether the client holds the lifetime id.
func (c *syncClient) holds(id syncstate.ID) bool {
	h, ok := c.held[id.Entity]
	return ok && c.id(h.Ref) == id
}

// id returns the full lifetime a Ref names in the current stream.
func (c *syncClient) id(r syncstate.Ref) syncstate.ID { return r.ID(c.session) }

// apply validates and applies one frame. It returns false, with no error, for a delta the client
// ignores: one that arrives before a baseline or on another stream.
//
//nolint:gocyclo,cyclop // One place for every check the client makes, in its order.
func (c *syncClient) apply(f syncstate.Frame) (bool, error) {
	if f.Open == nil && (!c.ready || f.Stream != c.stream) {
		return false, nil
	}
	if f.Open != nil {
		switch {
		case f.Open.Version != syncstate.ProtocolVersion || f.Open.Session == "":
			return false, fmt.Errorf("baseline opens version %d session %q", f.Open.Version, f.Open.Session)
		case c.fingerprint != "" && f.Open.Fingerprint != c.fingerprint:
			return false, fmt.Errorf("baseline fingerprint %q, contract %q", f.Open.Fingerprint, c.fingerprint)
		case f.Sequence != 1:
			return false, fmt.Errorf("baseline at sequence %d", f.Sequence)
		case len(f.Patches)+len(f.Removed) != 0:
			return false, fmt.Errorf("baseline with %d patches and %d removals", len(f.Patches), len(f.Removed))
		}
	} else if f.Sequence != c.sequence+1 {
		return false, fmt.Errorf("sequence %d after %d", f.Sequence, c.sequence)
	}
	held := c.held
	if f.Open != nil {
		held = nil // a baseline replaces everything held
	}
	index := func(i uint16) error {
		if int(i) >= c.components {
			return fmt.Errorf("component index %d, the contract has %d", i, c.components)
		}
		return nil
	}
	removed, recorded := map[uint32]bool{}, map[syncstate.Ref]bool{}
	for _, r := range f.Removed {
		if h, ok := held[r.Entity]; !ok || h.Ref != r || removed[r.Entity] {
			return false, fmt.Errorf("removes %v, which it does not hold", r)
		}
		removed[r.Entity] = true
	}
	for _, rec := range f.Records {
		h, ok := held[rec.Ref.Entity]
		switch {
		case recorded[rec.Ref]:
			return false, fmt.Errorf("records %v twice", rec.Ref)
		case ok && !removed[rec.Ref.Entity] && h.Ref != rec.Ref:
			return false, fmt.Errorf("records %v over %v without a removal", rec.Ref, h.Ref)
		case ok && !removed[rec.Ref.Entity] && rec.Revision <= h.Revision:
			return false, fmt.Errorf("records %v at revision %d after %d", rec.Ref, rec.Revision, h.Revision)
		}
		recorded[rec.Ref] = true
		for i, v := range rec.Components {
			if err := index(v.Component); err != nil {
				return false, err
			}
			if i > 0 && rec.Components[i-1].Component >= v.Component {
				return false, fmt.Errorf("record %v lists components out of order", rec.Ref)
			}
		}
	}
	patched := map[syncstate.Ref]bool{}
	for _, pa := range f.Patches {
		h, ok := held[pa.Ref.Entity]
		switch {
		case !ok || h.Ref != pa.Ref:
			return false, fmt.Errorf("patches %v, which it does not hold", pa.Ref)
		case removed[pa.Ref.Entity] || recorded[pa.Ref] || patched[pa.Ref]:
			return false, fmt.Errorf("patches %v, which the frame also removes, records or patches", pa.Ref)
		case pa.Revision <= h.Revision:
			return false, fmt.Errorf("patches %v to revision %d at %d", pa.Ref, pa.Revision, h.Revision)
		}
		patched[pa.Ref] = true
		seen := map[uint16]bool{}
		for _, i := range append(componentsOf(pa.Changed), pa.Dropped...) {
			if err := index(i); err != nil {
				return false, err
			}
			if seen[i] {
				return false, fmt.Errorf("patch %v names component %d twice", pa.Ref, i)
			}
			seen[i] = true
		}
	}
	holds := func(r syncstate.Ref) bool {
		h, ok := held[r.Entity]
		return recorded[r] || ok && h.Ref == r && !removed[r.Entity]
	}
	for _, g := range f.Access {
		if !holds(g.Ref) || g.Access == syncstate.None {
			return false, fmt.Errorf("grants %d for %v, which it does not hold", g.Access, g.Ref)
		}
	}

	if f.Open != nil {
		c.ready, c.session, c.stream, c.time, c.heartbeat = true, f.Open.Session, f.Stream, 0, f.Open.Heartbeat
		c.clear()
	}
	c.sequence = f.Sequence
	c.time += f.Time
	for _, r := range f.Removed {
		delete(c.held, r.Entity)
		delete(c.access, r.Entity)
	}
	for _, rec := range f.Records {
		if h, ok := c.held[rec.Ref.Entity]; !ok || h.Ref != rec.Ref {
			delete(c.access, rec.Ref.Entity)
		}
		c.held[rec.Ref.Entity] = rec
	}
	for _, pa := range f.Patches {
		c.held[pa.Ref.Entity] = merge(c.held[pa.Ref.Entity], pa)
	}
	for _, g := range f.Access {
		c.access[g.Ref.Entity] = g.Access
	}
	for n := range c.held {
		if c.access[n] == syncstate.None {
			return true, fmt.Errorf("holds %v without access", c.held[n].Ref)
		}
	}
	return true, nil
}

// merge applies a patch to a held record: changed components replace or add, dropped components
// go, the revision becomes the patch's, and the entity presence stays.
func merge(h syncstate.Record, pa syncstate.Patch) syncstate.Record {
	out := syncstate.Record{Ref: h.Ref, Revision: pa.Revision, Presence: h.Presence}
	for _, v := range h.Components {
		if !slices.Contains(pa.Dropped, v.Component) && !slices.Contains(componentsOf(pa.Changed), v.Component) {
			out.Components = append(out.Components, v)
		}
	}
	out.Components = append(out.Components, pa.Changed...)
	slices.SortFunc(out.Components, func(a, b syncstate.Value) int { return int(a.Component) - int(b.Component) })
	return out
}

func componentsOf(values []syncstate.Value) []uint16 {
	var out []uint16
	for _, v := range values {
		out = append(out, v.Component)
	}
	return out
}

// syncIndex is the wire index of a component: its position in the contract.
func syncIndex(t testing.TB, w *World, name string) uint16 {
	t.Helper()
	for i, c := range w.SyncContract().Components {
		if c.Name == name {
			return uint16(i)
		}
	}
	t.Fatalf("component %q is not in the contract", name)
	return 0
}

func refOf(e Entity) syncstate.Ref { return syncstate.RefOf(e.SyncID()) }

// emptyDelta reports whether a frame is a delta that changes nothing: a heartbeat.
func emptyDelta(f syncstate.Frame) bool {
	return f.Open == nil && len(f.Records)+len(f.Patches)+len(f.Removed)+len(f.Access) == 0
}

// requireHeartbeat asserts that frames are one empty delta: all that a reader with nothing to
// hear receives once its last frame is a heartbeat old, as it is at every TestWorld step.
func requireHeartbeat(t *testing.T, frames []syncstate.Frame, msgAndArgs ...any) {
	t.Helper()
	require.Len(t, frames, 1, msgAndArgs...)
	require.True(t, emptyDelta(frames[0]), msgAndArgs...)
}

func syncFrames(t *testing.T, sub *streamSubscriber) []syncstate.Frame {
	t.Helper()
	var frames []syncstate.Frame
	for _, e := range sub.queue {
		if e.response.GetEvent().GetName() != syncstate.EventName {
			continue
		}
		var frame syncstate.Frame
		require.NoError(t, msgpack.Unmarshal(e.response.GetEvent().GetPayload(), &frame))
		frames = append(frames, frame)
	}
	sub.queue = nil
	sub.queuedBytes = 0
	return frames
}

func TestSyncBaselinePrivacyAndDirtyPublication(t *testing.T) {
	w, sub := newSyncFixture(t, func(player string) bool { return player == "alice" })
	entity := w.Create[syncAvatar]()
	entity.Set(syncHealth{HP: 80, MaxHP: 100})
	entity.Set(syncPrivate{Secret: "not public"})
	w.Tick()
	frames := syncFrames(t, sub)
	require.Len(t, frames, 1)
	require.NotNil(t, frames[0].Open, "the first frame is a baseline")
	require.NotEmpty(t, frames[0].Open.Fingerprint)
	require.Equal(t, entity.SyncID().Session, frames[0].Open.Session)
	require.NotZero(t, frames[0].Stream)
	require.Len(t, frames[0].Records, 1)
	record := frames[0].Records[0]
	require.Equal(t, refOf(entity), record.Ref)
	require.Equal(t, []syncstate.Grant{{Ref: refOf(entity), Access: syncstate.View}}, frames[0].Access)
	require.Len(t, record.Components, 1)
	require.Equal(t, syncIndex(t, w.World, "health"), record.Components[0].Component)
	entity.Set(syncHealth{HP: 80, MaxHP: 100})
	entity.Set(syncPrivate{Secret: "changed"})
	w.Tick()
	requireHeartbeat(t, syncFrames(t, sub), "a private or equal write publishes nothing")
	entity.Set(syncHealth{HP: 50, MaxHP: 100})
	w.Tick()
	frames = syncFrames(t, sub)
	require.Len(t, frames, 1)
	require.Nil(t, frames[0].Open)
	require.Empty(t, frames[0].Records, "a reader that holds the entity gets a patch")
	require.Len(t, frames[0].Patches, 1)
	require.Equal(t, record.Revision+1, frames[0].Patches[0].Revision)
}

func TestSyncReuseAndPresenceNeverAlias(t *testing.T) {
	w, sub := newSyncFixture(t, func(string) bool { return true })
	// only returns the one frame the tick sent.
	only := func() syncstate.Frame {
		t.Helper()
		frames := syncFrames(t, sub)
		require.Len(t, frames, 1)
		return frames[0]
	}
	entity := w.Create[syncAvatar]()
	entity.Set(syncHealth{HP: 7})
	w.Tick()
	first := only().Records[0]
	entity.Remove[syncstate.Sync]()
	entity.Set(syncstate.Sync{})
	w.Tick()
	restated := only()
	require.Empty(t, restated.Patches)
	require.Len(t, restated.Records, 1, "a new entity presence restates the whole record")
	second := restated.Records[0]
	require.NotEqual(t, first.Presence, second.Presence)
	entity.Remove[syncHealth]()
	entity.Set(syncHealth{HP: 7})
	w.Tick()
	patched := only()
	require.Empty(t, patched.Records)
	require.Len(t, patched.Patches, 1, "the same data under a new component presence")
	third := patched.Patches[0]
	require.Len(t, third.Changed, 1)
	require.NotEqual(t, second.Components[0].Presence, third.Changed[0].Presence)
	entity.Destroy()
	next := w.Create[syncAvatar]()
	next.Set(syncHealth{HP: 7})
	w.Tick()
	frame := only()
	require.Equal(t, entity.ID(), next.ID())
	require.NotEqual(t, first.Ref, refOf(next))
	require.Equal(t, []syncstate.Ref{first.Ref}, frame.Removed)
	require.Equal(t, refOf(next), frame.Records[0].Ref)
}

func TestSyncCommitPrecedesOccurrenceAndRecoveryIsQuiet(t *testing.T) {
	w, sub := newSyncFixture(t, func(string) bool { return true })
	entity := w.Create[syncAvatar]()
	entity.Set(syncHealth{HP: 100})
	w.Tick()
	syncFrames(t, sub)
	entity.Set(syncHealth{HP: 0})
	w.Broadcast(syncHurt{Amount: 100})
	entity.Set(syncHealth{HP: 20})
	w.Tick()
	require.Len(t, sub.queue, 3)
	require.Equal(t, syncstate.EventName, sub.queue[0].response.GetEvent().GetName())
	require.Equal(t, "hurt", sub.queue[1].response.GetEvent().GetName())
	require.Equal(t, syncstate.EventName, sub.queue[2].response.GetEvent().GetName())
	frames := syncFrames(t, sub)
	require.Len(t, frames, 2)
	for _, f := range frames {
		require.Len(t, f.Patches, 1)
		require.Len(t, f.Patches[0].Changed, 1)
	}
	var health syncHealth
	require.NoError(t, msgpack.Unmarshal(frames[0].Patches[0].Changed[0].Data, &health))
	require.Zero(t, health.HP)
	require.NoError(t, msgpack.Unmarshal(frames[1].Patches[0].Changed[0].Data, &health))
	require.Equal(t, 20, health.HP)
	// A reconnect: the new stream gets a baseline and none of the tick's occurrences, which
	// belong to the stream that was open when they happened.
	reconnected := reconnectSync(t, w, "alice")
	w.Broadcast(syncHurt{Amount: 1})
	w.Tick()
	require.Len(t, reconnected.queue, 1)
	require.NotNil(t, syncFrames(t, reconnected)[0].Open)
}

// reconnectSync replaces player's stream with a new one subscribed to sync and hurt.
func reconnectSync(t *testing.T, w *TestWorld, player string) *streamSubscriber {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	sub := &streamSubscriber{
		ctx:        ctx,
		cancel:     cancel,
		events:     map[string]struct{}{"hurt": {}},
		queueReady: make(chan struct{}, 1),
	}
	addSubscriptions(sub, []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}})
	w.service.addSubscriber(player, sub)
	return sub
}

func TestSyncAudienceRevocation(t *testing.T) {
	admitted := false
	w, sub := newSyncFixture(t, func(string) bool { return admitted })
	w.Create[syncAvatar]()
	w.Broadcast(syncHurt{Amount: 1})
	w.Tick()
	require.Empty(t, sub.queue)
	require.NoError(t, context.Cause(sub.ctx))
}

func TestSyncResetAndRestoreReplaceEntityLifetimes(t *testing.T) {
	w, sub := newSyncFixture(t, func(string) bool { return true })
	entity := w.Create[syncAvatar]()
	entity.Set(syncHealth{HP: 90})
	w.Tick()
	old := syncFrames(t, sub)[0]
	snapshot, err := w.world.ToProto()
	require.NoError(t, err)
	require.NoError(t, w.world.FromProto(snapshot))
	w.Tick()
	frame := syncFrames(t, sub)[0]
	require.Equal(t, []syncstate.Ref{old.Records[0].Ref}, frame.Removed)
	require.NotEqual(t, old.Records[0].Ref, frame.Records[0].Ref)
	// A reset is ordinary transitions on the same stream: every lifetime it ended is removed.
	w.World.reset()
	w.Tick()
	frames := syncFrames(t, sub)
	require.Len(t, frames, 1)
	require.Nil(t, frames[0].Open)
	require.Equal(t, old.Stream, frames[0].Stream)
	require.Equal(t, frame.Sequence+1, frames[0].Sequence)
	require.Equal(t, []syncstate.Ref{frame.Records[0].Ref}, frames[0].Removed)
	require.Empty(t, frames[0].Records)
}

func TestSyncInitializationPublishesOnlyTheFinishedEntity(t *testing.T) {
	w, sub := newSyncFixture(t, func(string) bool { return true })
	w.Tick()
	syncFrames(t, sub)
	entity := w.Create[syncAvatar]()
	w.Broadcast(syncHurt{Amount: 1})
	entity.Set(syncHealth{HP: 100, MaxHP: 100})
	w.Tick()
	frames := syncFrames(t, sub)
	require.Len(t, frames, 1)
	var health syncHealth
	require.NoError(t, msgpack.Unmarshal(frames[0].Records[0].Components[0].Data, &health))
	require.Equal(t, 100, health.HP)
}

func TestSyncMetadataWithoutEnableKeepsLegacyEventDelivery(t *testing.T) {
	w := NewTestWorld(t, func(w *World) { w.RegisterComponent[syncHealth](); w.RegisterEvent[syncHurt]() })
	w.World.events.RegisterHandler(event.KindDefault, w.service.publishDefaultEvent)
	ctx, cancel := context.WithCancelCause(t.Context())
	sub := &streamSubscriber{
		ctx:        ctx,
		cancel:     cancel,
		events:     map[string]struct{}{"hurt": {}},
		queueReady: make(chan struct{}, 1),
	}
	w.service.addSubscriber("balance", sub)
	w.SendTo("balance", syncHurt{Amount: 3})
	w.Tick()
	require.Len(t, sub.queue, 1)
	require.Equal(t, "hurt", sub.queue[0].response.GetEvent().GetName())
}

func TestSyncNewSubscriberCannotReceiveBeforeTickBaseline(t *testing.T) {
	w, old := newSyncFixture(t, func(string) bool { return true })
	w.Tick()
	syncFrames(t, old)
	sub := reconnectSync(t, w, "bob")
	w.Broadcast(syncHurt{Amount: 1}) // the tick bob joins: bob's stream opens after its events
	w.Tick()
	require.Len(t, sub.queue, 1)
	require.NotNil(t, syncFrames(t, sub)[0].Open)
	w.Broadcast(syncHurt{Amount: 2})
	w.Tick()
	require.Len(t, sub.queue, 2)
	require.Equal(t, "hurt", sub.queue[0].response.GetEvent().GetName())
	requireHeartbeat(t, syncFrames(t, sub), "the tick's events come before its heartbeat")
}

func TestSyncDisconnectedOccurrenceAndReplyOnlyCallerAreDenied(t *testing.T) {
	w, _ := newSyncFixture(t, func(string) bool { return true })
	w.Tick()
	reply := make(chan *iscv1.Event, 1)
	w.service.replyWaiters["hurt"] = map[string][]chan *iscv1.Event{"offline": {reply}}
	require.NotPanics(t, func() {
		w.SendTo("offline", syncHurt{Amount: 1})
		w.Tick()
	})
	require.Empty(t, reply)
}

type syncReply struct{ Code string }

func (syncReply) Name() string { return "reply" }

func TestSyncRevocationClearsStateWithoutDroppingCleanupOrRefusal(t *testing.T) {
	admitted := true
	w, sub := newSyncFixture(t, func(string) bool { return admitted })
	sub.events["reply"] = struct{}{}
	w.Create[syncAvatar]()
	w.Tick()
	syncFrames(t, sub)
	admitted = false
	w.SendTo("alice", syncReply{Code: "session ended"})
	w.Tick()
	require.NoError(t, context.Cause(sub.ctx))
	// The tick's events come first; the close that clears the view follows them.
	require.Len(t, sub.queue, 2)
	require.Equal(t, "reply", sub.queue[0].response.GetEvent().GetName())
	require.Equal(t, syncstate.EventName, sub.queue[1].response.GetEvent().GetName())
	frames := syncFrames(t, sub)
	require.NotNil(t, frames[0].Open, "the cleared view is an empty baseline on a new stream")
	require.Empty(t, frames[0].Records)
	require.Zero(t, frames[0].Open.Heartbeat, "a revoked reader is promised no heartbeat")
	reply := make(chan *iscv1.Event, 1)
	w.service.replyWaiters["reply"] = map[string][]chan *iscv1.Event{"outsider": {reply}}
	require.NoError(
		t,
		w.service.publishDefaultEvent(
			t.Context(),
			event.Event{Kind: event.KindDefault, Recipient: "outsider", Payload: syncReply{Code: "not in session"}},
		),
	)
	require.Len(t, reply, 1)
	admitted = true
	w.Tick()
	restored := syncFrames(t, sub)
	require.Len(t, restored, 1)
	require.NotNil(t, restored[0].Open)
	require.Len(t, restored[0].Records, 1)
	require.Equal(t, uint64(1000), restored[0].Open.Heartbeat, "a readmitted reader is promised one again")
	require.NotEqual(t, frames[0].Stream, restored[0].Stream)
	w.Tick()
	requireHeartbeat(t, syncFrames(t, sub))
}

type syncZeta struct {
	syncstate.State

	Z int
}

func (syncZeta) Name() string { return "Zeta" }

type syncAlpha struct {
	syncstate.State

	A int
}

func (syncAlpha) Name() string { return "alpha" }

type syncBeta struct {
	syncstate.State

	B int
}

func (syncBeta) Name() string { return "beta" }

type syncLettered struct {
	syncstate.Sync

	Beta  syncBeta
	Zeta  syncZeta
	Alpha syncAlpha
}

// A Value's Component indexes the components in the order Contract.CanonicalJSON lists them:
// byte order of names, whatever the registration order. "Zeta" sorts before "alpha" only by
// bytes, not by letters.
func TestSyncComponentIndexIsContractOrder(t *testing.T) {
	register := func(w *World) {
		w.RegisterComponent[syncBeta]()
		w.RegisterComponent[syncZeta]()
		w.RegisterComponent[syncAlpha]()
	}
	exported, err := ExportSyncContract(register)
	require.NoError(t, err)
	var contract syncstate.Contract
	require.NoError(t, json.Unmarshal(exported, &contract))
	w := NewTestWorld(t, func(w *World) { register(w); w.EnableSync(func(string) bool { return true }) })
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	sub := &streamSubscriber{
		ctx:        ctx,
		cancel:     cancel,
		events:     map[string]struct{}{syncstate.EventName: {}},
		queueReady: make(chan struct{}, 1),
	}
	w.service.addSubscriber("observer", sub)
	e := w.Create[syncLettered]()
	e.Set(syncBeta{B: 2})
	e.Set(syncZeta{Z: 26})
	e.Set(syncAlpha{A: 1})
	w.Tick()
	frames := syncFrames(t, sub)
	require.Len(t, frames, 1)
	require.Len(t, frames[0].Records, 1)
	values := frames[0].Records[0].Components
	require.Len(t, values, 3)
	want := map[string]map[string]int64{"Zeta": {"Z": 26}, "alpha": {"A": 1}, "beta": {"B": 2}}
	var names []string
	for _, v := range values {
		require.Less(t, int(v.Component), len(contract.Components))
		component := contract.Components[v.Component]
		names = append(names, component.Name)
		var data map[string]int64
		require.NoError(t, msgpack.Unmarshal(v.Data, &data))
		require.Equal(t, want[component.Name], data, "component %d is %s in the contract", v.Component, component.Name)
		require.Equal(t, component.Fields[0].Name, slices.Collect(maps.Keys(data))[0])
	}
	require.Equal(t, []string{"Zeta", "alpha", "beta"}, names)
}
