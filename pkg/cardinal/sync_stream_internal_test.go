package cardinal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	gosync "sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// Exercises real HTTP/2 streams. Sync capture and dispatch run through the same
// production tick used by TestWorld; neither client reads the subscriber queue.
func TestSyncTwoHTTP2ClientsReconnectAndReadmission(t *testing.T) {
	allowAlice := true
	w := NewTestWorld(t, func(w *World) {
		w.RegisterComponent[syncLoadout]()
		w.RegisterComponent[syncCosmetics]()
		w.RegisterComponent[syncPrivate]()
		w.EnableSync(func(player string) bool { return player == "bob" || player == "alice" && allowAlice })
	})
	aliceClient, bobClient := newSyncStreamHarness(t, w)
	e := w.Create[syncEquipment]()
	e.Set(syncLoadout{WeaponID: "wand"})
	e.Set(syncCosmetics{Tint: 123, Opacity: 0.5})
	e.Set(syncPrivate{Secret: "never on the wire"})
	// Unity starts an empty stream, then subscribes through a separate RPC.
	alice := aliceClient.open(t, "alice")
	alice.read()
	aliceClient.subscribe(t, "alice", syncstate.EventName)
	bob := bobClient.open(t, "bob", syncstate.EventName)
	bob.read()
	w.Tick()
	a := receiveSyncFrame(t, alice)
	b := receiveSyncFrame(t, bob)
	require.True(t, a.Baseline)
	require.True(t, b.Baseline)
	require.NotEqual(t, a.Stream, b.Stream)
	require.Equal(t, a.Records, b.Records)
	require.Len(t, a.Records, 1)
	var names []string
	for _, component := range a.Records[0].Components {
		names = append(names, component.Name)
	}
	require.ElementsMatch(t, []string{"loadout", "cosmetics"}, names, "private component is omitted")
	require.Equal(t, e.SyncID(), a.Records[0].ID)

	e.Set(syncLoadout{WeaponID: "sword", Ammo: 7, Equipped: true})
	w.Tick()
	aUpdate := receiveSyncFrame(t, alice)
	bUpdate := receiveSyncFrame(t, bob)
	require.False(t, aUpdate.Baseline)
	require.False(t, bUpdate.Baseline)
	require.Equal(t, aUpdate.Records, bUpdate.Records)
	require.Equal(t, uint16(7), streamLoadout(t, bUpdate).Ammo)

	alice.cancel()
	select {
	case <-alice.done:
	case <-time.After(5 * time.Second):
		t.Fatal("disconnected reader did not stop")
	}
	e.Set(syncLoadout{WeaponID: "bow", Ammo: 9})
	w.Tick()
	bOffline := receiveSyncFrame(t, bob)
	require.Equal(t, b.Stream, bOffline.Stream)
	require.Equal(t, uint16(9), streamLoadout(t, bOffline).Ammo)

	alice = aliceClient.open(t, "alice")
	alice.read()
	aliceClient.subscribe(t, "alice", syncstate.EventName)
	w.Tick() // No state change: reconnect still needs a complete baseline.
	rejoined := receiveSyncFrame(t, alice)
	require.True(t, rejoined.Baseline)
	require.NotEqual(t, a.Stream, rejoined.Stream)
	require.Equal(t, bOffline.Records, rejoined.Records)

	allowAlice = false
	w.Tick()
	revoked := receiveSyncFrame(t, alice)
	require.True(t, revoked.Baseline)
	require.NotEqual(t, rejoined.Stream, revoked.Stream)
	require.Empty(t, revoked.Records)
	e.Set(syncLoadout{WeaponID: "staff", Ammo: 11})
	w.Tick()
	require.Equal(t, uint16(11), streamLoadout(t, receiveSyncFrame(t, bob)).Ammo)
	allowAlice = true
	w.Tick()
	readmitted := receiveSyncFrame(t, alice)
	// If a suspended-segment update leaked, it would arrive before this baseline.
	require.True(t, readmitted.Baseline)
	require.NotEqual(t, revoked.Stream, readmitted.Stream)
	require.Equal(t, syncLoadout{WeaponID: "staff", Ammo: 11}, streamLoadout(t, readmitted))

	e.Set(syncLoadout{WeaponID: "staff", Ammo: 12})
	w.Tick()
	require.Equal(t, uint16(12), streamLoadout(t, receiveSyncFrame(t, alice)).Ammo)
	lastBob := receiveSyncFrame(t, bob)
	require.False(t, lastBob.Baseline, "the uninterrupted client never receives the other client's baselines")
	require.Equal(t, b.Stream, lastBob.Stream)
	require.Equal(t, uint16(12), streamLoadout(t, lastBob).Ammo)
}

func newSyncStreamHarness(t *testing.T, w *TestWorld) (*streamHarness, *streamHarness) {
	t.Helper()
	w.World.events.RegisterHandler(event.KindDefault, w.service.publishDefaultEvent)
	mux, err := w.service.newMux(authenticatorDev{}.authenticate)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(mux)
	server.Config.Protocols = h2cProtocols()
	server.Start()
	t.Cleanup(func() { server.CloseClientConnections(); server.Close() })
	newClient := func() *streamHarness {
		protocols := new(http.Protocols)
		protocols.SetUnencryptedHTTP2(true)
		transport := &http.Transport{Protocols: protocols}
		t.Cleanup(transport.CloseIdleConnections)
		h := &streamHarness{fixture: &serviceFixture{svc: w.service, world: w.World}}
		h.client = cardinalv1connect.NewCardinalServiceClient(
			&http.Client{Transport: transport},
			server.URL,
			connect.WithGRPC(),
		)
		h.ctx, h.cancel = context.WithCancel(context.Background())
		t.Cleanup(h.cancel)
		return h
	}
	return newClient(), newClient() // Separate transports, as on two machines.
}

func receiveSyncFrame(t *testing.T, client *clientStream) syncstate.Frame {
	t.Helper()
	select {
	case msg := <-client.messages:
		require.NotNil(t, msg)
		require.Equal(t, syncstate.EventName, msg.GetEvent().GetName())
		var frame syncstate.Frame
		require.NoError(t, msgpack.Unmarshal(msg.GetEvent().GetPayload(), &frame))
		return frame
	case <-client.done:
		t.Fatalf("sync stream ended early: %v", client.err)
		return syncstate.Frame{}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for sync over HTTP/2")
		return syncstate.Frame{}
	}
}

func streamLoadout(t *testing.T, frame syncstate.Frame) syncLoadout {
	t.Helper()
	require.Len(t, frame.Records, 1)
	for _, component := range frame.Records[0].Components {
		if component.Name == "loadout" {
			var value syncLoadout
			require.NoError(t, msgpack.Unmarshal(component.Data, &value))
			return value
		}
	}
	t.Fatal("missing loadout")
	return syncLoadout{}
}

// Subscriptions arrive on RPC goroutines while the tick publishes. The tick reads them only at
// reconcile, under service.mu; run with -race. Every stream any subscriber saw must pass the
// client's checks, and the streams open at the end must hold the world.
func TestSyncSubscriptionsRaceTheTick(t *testing.T) {
	w := NewTestWorld(t, func(w *World) {
		w.RegisterComponent[syncHealth]()
		w.RegisterComponent[syncPrivate]()
		w.EnableSync(func(string) bool { return true })
	})
	var mu gosync.Mutex
	var created []*streamSubscriber
	stop := make(chan struct{})
	var wg gosync.WaitGroup
	for i := range 6 {
		player := fmt.Sprintf("p%d", i)
		wg.Go(func() {
			subscription := []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}}
			for j := 0; ; j++ {
				select {
				case <-stop:
					return
				default:
				}
				ctx, cancel := context.WithCancelCause(context.Background())
				sub := &streamSubscriber{
					ctx:        ctx,
					cancel:     cancel,
					events:     map[string]struct{}{},
					queueReady: make(chan struct{}, 1),
				}
				addSubscriptions(sub, subscription)
				mu.Lock()
				created = append(created, sub)
				mu.Unlock()
				w.service.addSubscriber(player, sub)
				if j%3 == 0 { // unsubscribe and subscribe again on the same stream
					w.service.mu.Lock()
					removeSubscriptions(sub, subscription)
					w.service.mu.Unlock()
					w.service.mu.Lock()
					addSubscriptions(sub, subscription)
					w.service.mu.Unlock()
				}
				if j%4 == 0 {
					w.service.removeSubscriber(player, sub)
				}
				time.Sleep(time.Duration(j%3) * 100 * time.Microsecond)
			}
		})
	}
	var live []Entity
	for i := range 300 {
		if len(live) < 8 {
			e := w.Create[syncAvatar]()
			live = append(live, e)
		}
		live[i%len(live)].Set(syncHealth{HP: i})
		if i%7 == 0 {
			live[0].Destroy()
			live = live[1:]
		}
		w.Tick()
	}
	close(stop)
	wg.Wait()
	// Every stream any subscriber saw passes the client's checks.
	for _, sub := range created {
		checkSyncStream(t, sub)
	}
	// Streams opened after the churn hold the world.
	var final []*streamSubscriber
	for i := range 6 {
		ctx, cancel := context.WithCancelCause(t.Context())
		sub := &streamSubscriber{ctx: ctx, cancel: cancel, events: map[string]struct{}{}, queueReady: make(chan struct{}, 1)}
		addSubscriptions(sub, []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}})
		w.service.addSubscriber(fmt.Sprintf("p%d", i), sub)
		final = append(final, sub)
	}
	w.Tick()
	want := map[uint32]syncstate.ID{}
	for _, e := range live {
		want[uint32(e.ID())] = e.SyncID()
	}
	for _, sub := range final {
		held, ok := checkSyncStream(t, sub)
		require.True(t, ok, "an open stream has a baseline")
		require.Equal(t, want, held)
	}
}

// checkSyncStream replays one subscriber's sync frames with the client's checks and returns
// what the last stream holds.
func checkSyncStream(t *testing.T, sub *streamSubscriber) (map[uint32]syncstate.ID, bool) {
	t.Helper()
	var stream string
	var sequence uint64
	held := map[uint32]syncstate.ID{}
	for _, q := range sub.queue {
		var f syncstate.Frame
		require.NoError(t, msgpack.Unmarshal(q.response.GetEvent().GetPayload(), &f))
		if f.Baseline {
			stream, sequence, held = f.Stream, f.Sequence, map[uint32]syncstate.ID{}
		} else {
			require.NotEmpty(t, stream, "a delta before any baseline")
			if f.Stream != stream {
				continue // an old stream after a resubscribe: the client ignores it
			}
			require.Equal(t, sequence+1, f.Sequence)
			sequence = f.Sequence
		}
		for _, id := range f.Removed {
			require.Equal(t, id, held[id.Entity])
			delete(held, id.Entity)
		}
		for _, r := range f.Records {
			held[r.ID.Entity] = r.ID
		}
	}
	return held, stream != ""
}
