package cardinal

import (
	"context"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunLoopRestartPreservesHeight drives the real run loop (its deferred final snapshot is the
// production shutdown path), shuts it down cleanly via context cancellation, then restores into a
// FRESH World sharing the same storage, asserting the running height is preserved across the
// restart boundary. (Procedure F in the test plan.)
func TestRunLoopRestartPreservesHeight(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	storage := &memSnapshotStorage{t: t}

	debugOn := true
	w1, err := NewWorld(WorldOptions{
		Region: "e2e-restart", Organization: "e2e-restart", Project: "e2e-restart", ShardID: "0",
		TickRate: 1000, SnapshotStorageType: snapshot.StorageTypeNop, SnapshotRate: 1, Debug: &debugOn,
	})
	require.NoError(t, err)
	w1.useSyncSnapshotStorage(storage)

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- w1.run(ctx) }()

	// Pause deterministically (loop is selecting on pauseCh while not paused).
	pauseReply := make(chan uint64, 1)
	w1.debug.control.pauseCh <- pauseReply
	require.Equal(t, uint64(0), <-pauseReply, "pause should report the pre-tick height 0")

	// Step exactly 5 times.
	for i := range 5 {
		stepReply := make(chan uint64, 1)
		w1.debug.control.stepCh <- stepReply
		h := <-stepReply
		require.Equal(t, uint64(i+1), h, "step %d should report height %d", i+1, i+1)
	}

	// Clean shutdown while paused: cancel so run returns and the final-snapshot defer fires.
	cancel()
	require.ErrorIs(t, <-runErr, context.Canceled, "run should return context.Canceled on shutdown")

	// Boot a fresh World sharing the same storage and restore (simulates a real restart).
	debugOff := false
	w2, err := NewWorld(WorldOptions{
		Region: "e2e-restart", Organization: "e2e-restart", Project: "e2e-restart", ShardID: "0",
		TickRate: 1000, SnapshotStorageType: snapshot.StorageTypeNop, SnapshotRate: 1, Debug: &debugOff,
	})
	require.NoError(t, err)
	w2.useSyncSnapshotStorage(storage)
	w2.world.Init()
	require.NoError(t, w2.restore(context.Background()))
	assert.Equal(t, uint64(5), w2.currentTick.height,
		"after 5 ticks via the real run loop + one clean restart, height should be 5, got %d", w2.currentTick.height)
}

// TestRunLoopZeroTicksStaysAtZero drives the real run loop, shuts down with ZERO ticks run, then
// restores into a fresh World, asserting the height stays 0 (does not drift to 1).
func TestRunLoopZeroTicksStaysAtZero(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	storage := &memSnapshotStorage{t: t}

	debugOn := true
	w1, err := NewWorld(WorldOptions{
		Region: "e2e-zero", Organization: "e2e-zero", Project: "e2e-zero", ShardID: "0",
		TickRate: 1000, SnapshotStorageType: snapshot.StorageTypeNop, SnapshotRate: 1, Debug: &debugOn,
	})
	require.NoError(t, err)
	w1.useSyncSnapshotStorage(storage)

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- w1.run(ctx) }()

	pauseReply := make(chan uint64, 1)
	w1.debug.control.pauseCh <- pauseReply
	require.Equal(t, uint64(0), <-pauseReply)

	// Shut down immediately (zero steps).
	cancel()
	require.ErrorIs(t, <-runErr, context.Canceled)

	// No snapshot should have been written at height 0.
	_, loadErr := storage.Load(context.Background())
	require.ErrorIs(t, loadErr, snapshot.ErrSnapshotNotFound, "zero-tick run must not write a snapshot")

	debugOff := false
	w2, err := NewWorld(WorldOptions{
		Region: "e2e-zero", Organization: "e2e-zero", Project: "e2e-zero", ShardID: "0",
		TickRate: 1000, SnapshotStorageType: snapshot.StorageTypeNop, SnapshotRate: 1, Debug: &debugOff,
	})
	require.NoError(t, err)
	w2.useSyncSnapshotStorage(storage)
	w2.world.Init()
	require.NoError(t, w2.restore(context.Background()))
	assert.Equal(t, uint64(0), w2.currentTick.height,
		"after zero ticks via the real run loop + one restart, height should stay 0, got %d", w2.currentTick.height)
}

// TestCrashMidTickRestoresCorrectly simulates a crash after persistState ran (pre-increment label)
// but before the deferred final snapshot, then restores. Validates that a persistState-labeled
// (pre-increment) snapshot restores to the correct running height with restore's +1.
// (Procedure G in the test plan.)
func TestCrashMidTickRestoresCorrectly(t *testing.T) {
	w := newTestWorld(t) // SnapshotRate=1, inits
	for range 3 {
		w.Tick(time.Now())
	}
	// After 3 ticks, height is 3. persistState wrote pre-increment labels 0, 1, 2 (last stored = 2).
	// Simulate a crash before the deferred final snapshot: do NOT call writeFinalSnapshot.
	require.NoError(t, w.restore(context.Background()))
	assert.Equal(t, uint64(3), w.currentTick.height,
		"a persistState (pre-increment=2) snapshot from tick 3 should restore to height 3, got %d", w.currentTick.height)
}

// TestRepeatRestartHeightStable ticks the world once, then repeats the clean restart cycle
// (writeFinalSnapshot + restore, no new ticks) 4 times, asserting the height does not creep +1
// per restart when no further work is done — it must stay pinned at the last completed height.
func TestRepeatRestartHeightStable(t *testing.T) {
	w := newTestWorld(t)
	for range 3 {
		w.Tick(time.Now())
	}
	const want uint64 = 3
	for r := range 4 {
		w.writeFinalSnapshot()
		require.NoError(t, w.restore(context.Background()))
		assert.Equal(t, want, w.currentTick.height,
			"after restart %d (no new ticks), height should stay %d, got %d", r+1, want, w.currentTick.height)
	}
}
