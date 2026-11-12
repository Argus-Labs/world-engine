package cardinal

import (
	"context"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"testing"
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
	sub := &streamSubscriber{ctx: ctx, cancel: cancel, events: map[string]struct{}{syncstate.EventName: {}, "hurt": {}}, queueReady: make(chan struct{}, 1)}
	w.service.addSubscriber("alice", sub)
	return w, sub
}

func syncFrames(t *testing.T, sub *streamSubscriber) []syncstate.Frame {
	t.Helper()
	var frames []syncstate.Frame
	for _, e := range sub.queue {
		if e.response.Event.Name != syncstate.EventName {
			continue
		}
		var frame syncstate.Frame
		require.NoError(t, msgpack.Unmarshal(e.response.Event.Payload, &frame))
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
	require.True(t, frames[0].Baseline)
	require.NotEmpty(t, frames[0].Fingerprint)
	require.NotEmpty(t, frames[0].Stream)
	require.Len(t, frames[0].Records, 1)
	record := frames[0].Records[0]
	require.Equal(t, entity.SyncID(), record.ID)
	require.Equal(t, []syncstate.Grant{{ID: entity.SyncID(), Access: syncstate.View}}, frames[0].Access)
	require.Len(t, record.Components, 1)
	require.Equal(t, "health", record.Components[0].Name)
	entity.Set(syncHealth{HP: 80, MaxHP: 100})
	entity.Set(syncPrivate{Secret: "changed"})
	w.Tick()
	require.Empty(t, syncFrames(t, sub))
	entity.Set(syncHealth{HP: 50, MaxHP: 100})
	w.Tick()
	frames = syncFrames(t, sub)
	require.Len(t, frames, 1)
	require.False(t, frames[0].Baseline)
	require.Equal(t, record.Revision+1, frames[0].Records[0].Revision)
}

func TestSyncReuseAndPresenceNeverAlias(t *testing.T) {
	w, sub := newSyncFixture(t, func(string) bool { return true })
	entity := w.Create[syncAvatar]()
	entity.Set(syncHealth{HP: 7})
	w.Tick()
	first := syncFrames(t, sub)[0].Records[0]
	entity.Remove[syncstate.Sync]()
	entity.Set(syncstate.Sync{})
	w.Tick()
	second := syncFrames(t, sub)[0].Records[0]
	require.NotEqual(t, first.Presence, second.Presence)
	entity.Remove[syncHealth]()
	entity.Set(syncHealth{HP: 7})
	w.Tick()
	third := syncFrames(t, sub)[0].Records[0]
	require.NotEqual(t, second.Components[0].Presence, third.Components[0].Presence)
	entity.Destroy()
	next := w.Create[syncAvatar]()
	next.Set(syncHealth{HP: 7})
	w.Tick()
	frame := syncFrames(t, sub)[0]
	require.Equal(t, entity.ID(), next.ID())
	require.NotEqual(t, first.ID, next.SyncID())
	require.Equal(t, []syncstate.ID{first.ID}, frame.Removed)
	require.Equal(t, next.SyncID(), frame.Records[0].ID)
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
	require.Equal(t, syncstate.EventName, sub.queue[0].response.Event.Name)
	require.Equal(t, "hurt", sub.queue[1].response.Event.Name)
	require.Equal(t, syncstate.EventName, sub.queue[2].response.Event.Name)
	frames := syncFrames(t, sub)
	require.Len(t, frames, 2)
	var health syncHealth
	require.NoError(t, msgpack.Unmarshal(frames[0].Records[0].Components[0].Data, &health))
	require.Zero(t, health.HP)
	require.NoError(t, msgpack.Unmarshal(frames[1].Records[0].Components[0].Data, &health))
	require.Equal(t, 20, health.HP)
	// A reconnect: the new stream gets a baseline and none of the tick's occurrences, which
	// belong to the stream that was open when they happened.
	reconnected := reconnectSync(t, w, "alice")
	w.Broadcast(syncHurt{Amount: 1})
	w.Tick()
	require.Len(t, reconnected.queue, 1)
	require.True(t, syncFrames(t, reconnected)[0].Baseline)
}

// reconnectSync replaces player's stream with a new one subscribed to sync and hurt.
func reconnectSync(t *testing.T, w *TestWorld, player string) *streamSubscriber {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	sub := &streamSubscriber{ctx: ctx, cancel: cancel, events: map[string]struct{}{"hurt": {}}, queueReady: make(chan struct{}, 1)}
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
	require.Equal(t, []syncstate.ID{old.Records[0].ID}, frame.Removed)
	require.NotEqual(t, old.Records[0].ID, frame.Records[0].ID)
	// A reset is ordinary transitions on the same stream: every lifetime it ended is removed.
	w.World.reset()
	w.Tick()
	frames := syncFrames(t, sub)
	require.Len(t, frames, 1)
	require.False(t, frames[0].Baseline)
	require.Equal(t, old.Stream, frames[0].Stream)
	require.Equal(t, frame.Sequence+1, frames[0].Sequence)
	require.Equal(t, []syncstate.ID{frame.Records[0].ID}, frames[0].Removed)
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
	sub := &streamSubscriber{ctx: ctx, cancel: cancel, events: map[string]struct{}{"hurt": {}}, queueReady: make(chan struct{}, 1)}
	w.service.addSubscriber("balance", sub)
	w.SendTo("balance", syncHurt{Amount: 3})
	w.Tick()
	require.Len(t, sub.queue, 1)
	require.Equal(t, "hurt", sub.queue[0].response.Event.Name)
}

func TestSyncNewSubscriberCannotReceiveBeforeTickBaseline(t *testing.T) {
	w, old := newSyncFixture(t, func(string) bool { return true })
	w.Tick()
	syncFrames(t, old)
	sub := reconnectSync(t, w, "bob")
	w.Broadcast(syncHurt{Amount: 1}) // the tick bob joins: bob's stream opens after its events
	w.Tick()
	require.Len(t, sub.queue, 1)
	require.True(t, syncFrames(t, sub)[0].Baseline)
	w.Broadcast(syncHurt{Amount: 2})
	w.Tick()
	require.Len(t, sub.queue, 1)
	require.Equal(t, "hurt", sub.queue[0].response.GetEvent().GetName())
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
	require.True(t, frames[0].Baseline)
	require.Empty(t, frames[0].Records)
	reply := make(chan *iscv1.Event, 1)
	w.service.replyWaiters["reply"] = map[string][]chan *iscv1.Event{"outsider": {reply}}
	require.NoError(t, w.service.publishDefaultEvent(t.Context(), event.Event{Kind: event.KindDefault, Recipient: "outsider", Payload: syncReply{Code: "not in session"}}))
	require.Len(t, reply, 1)
	admitted = true
	w.Tick()
	restored := syncFrames(t, sub)
	require.Len(t, restored, 1)
	require.True(t, restored[0].Baseline)
	require.Len(t, restored[0].Records, 1)
	require.NotEqual(t, frames[0].Stream, restored[0].Stream)
	w.Tick()
	require.Empty(t, sub.queue)
}
