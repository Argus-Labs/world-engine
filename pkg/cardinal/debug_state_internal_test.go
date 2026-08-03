package cardinal

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// newDebugStateWorld builds a real World with debug on, which is the only configuration in which
// DebugService.GetState is reachable: its handler is mounted only when w.debug != nil (see
// service.init). NewWorld is used rather than a hand-assembled World so the empty state it seeds is
// the one under test.
func newDebugStateWorld(t *testing.T) (*World, *snapshotEntities) {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")

	debug := true
	w, err := NewWorld(WorldOptions{
		Region:              "debug-state",
		Organization:        "debug-state",
		Project:             "debug-state",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        5,
		Debug:               &debug,
	})
	require.NoError(t, err)
	require.NotNil(t, w.debug, "debug module must exist, otherwise GetState is not mounted")

	state := &snapshotEntities{}
	require.NoError(t, initSystemFields(state, w))
	w.world.Init()

	return w, state
}

// getState calls the RPC handler exactly as the ConnectRPC layer would.
func getState(t *testing.T, w *World) *cardinalv1.GetStateResponse {
	t.Helper()

	resp, err := w.debug.GetState(context.Background(), connect.NewRequest(&cardinalv1.GetStateRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp.Msg
}

// TestDebugGetStateIsServableBeforeFirstTick pins the always-servable half of the GetState
// contract: nothing has published yet, so the answer comes from the seed installed by NewWorld.
func TestDebugGetStateIsServableBeforeFirstTick(t *testing.T) {
	w, _ := newDebugStateWorld(t)

	msg := getState(t, w)
	snap := msg.GetSnapshot()
	require.NotNil(t, snap, "GetState must never return a nil snapshot")
	require.NotNil(t, snap.GetWorldState(), "GetState must never return a nil world state")
	assert.Zero(t, snap.GetTickHeight())
	assert.Empty(t, snap.GetWorldState().GetArchetypes())
	assert.False(t, msg.GetIsPaused())
}

// TestDebugGetStatePublishesEveryTick covers the other two halves of the contract: the published
// snapshot carries the height of the tick that just finished, and once published it is frozen — a
// later tick swaps in a new envelope instead of mutating the one a caller is already holding.
func TestDebugGetStatePublishesEveryTick(t *testing.T) {
	w, state := newDebugStateWorld(t)
	ctx := context.Background()

	seedSnapshotWorld(t, state)

	for range 12 {
		held := getState(t, w).GetSnapshot()
		require.NotNil(t, held)
		frozen, err := proto.MarshalOptions{Deterministic: true}.Marshal(held)
		require.NoError(t, err)

		// Change the world so the next publish cannot coincidentally match the held one.
		_, e := state.Entities.Create()
		e.Position.Set(Position3D{X: float64(w.currentTick.height)})

		completed := w.currentTick.height
		w.Tick(ctx, time.Now())

		snap := getState(t, w).GetSnapshot()
		require.NotNil(t, snap)
		assert.Equal(t, completed, snap.GetTickHeight(),
			"published state must describe the tick that just finished")
		assert.NotEmpty(t, snap.GetWorldState().GetArchetypes(),
			"published state must be the live world, not the empty seed")

		after, err := proto.MarshalOptions{Deterministic: true}.Marshal(held)
		require.NoError(t, err)
		assert.Equal(t, frozen, after, "ticking mutated a snapshot that was already published")
	}
}

// TestDebugGetStateConcurrentWithTicks is the race-detector gate for the publish: GetState is
// served on an RPC goroutine while the tick loop republishes, and the response graph is serialized
// after the handler returns. Run with -race.
func TestDebugGetStateConcurrentWithTicks(t *testing.T) {
	w, state := newDebugStateWorld(t)
	ctx := context.Background()

	seedSnapshotWorld(t, state)

	const readers = 4
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				resp, err := w.debug.GetState(ctx, connect.NewRequest(&cardinalv1.GetStateRequest{}))
				if err != nil {
					t.Errorf("GetState failed: %v", err)
					return
				}
				snap := resp.Msg.GetSnapshot()
				if snap == nil {
					t.Error("GetState returned a nil snapshot")
					return
				}
				// Serializing is what the ConnectRPC layer does with the response, and it reads
				// the whole graph the tick goroutine just built.
				if _, err := proto.Marshal(snap); err != nil {
					t.Errorf("failed to serialize the published state: %v", err)
					return
				}
			}
		}()
	}

	for range 100 {
		w.Tick(ctx, time.Now())
	}
	close(stop)
	wg.Wait()

	assert.Equal(t, uint64(100), w.currentTick.height)
}
