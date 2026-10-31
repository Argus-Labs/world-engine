package cardinal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
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
		h.client = cardinalv1connect.NewCardinalServiceClient(&http.Client{Transport: transport}, server.URL, connect.WithGRPC())
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
