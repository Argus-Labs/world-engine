package cardinal

import (
	"context"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestWorld builds a minimal, isolated World backed by the in-memory snapshot storage and a
// synchronous writer. Debug stepping lets tests drive shutdown at an exact tick height.
func newTestWorld(t *testing.T) *World {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")

	debug := true
	w, err := NewWorld(WorldOptions{
		Region:              "bug-repro",
		Organization:        "bug-repro",
		Project:             "bug-repro",
		ShardID:             "0",
		TickRate:            1,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1,
		Debug:               &debug,
	})
	require.NoError(t, err)

	// Swap the nop storage for one that keeps the last snapshot and validates its envelope, and
	// write to it synchronously so the test observes every Write immediately.
	w.useSyncSnapshotStorage(&memSnapshotStorage{t: t})

	return w
}

// runSnapshotTicks runs a fixed number of ticks, then exercises run's shutdown defer.
func runSnapshotTicks(t *testing.T, w *World, ticks int) {
	t.Helper()
	w.debug.setPaused(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- w.run(ctx) }()
	for range ticks {
		reply := make(chan uint64, 1)
		select {
		case w.debug.control.stepCh <- reply:
			<-reply
		case err := <-runErr:
			t.Fatalf("run stopped before stepping: %v", err)
		}
	}
	cancel()
	require.ErrorIs(t, <-runErr, context.Canceled)
}

// TestRestoreDriftsTickHeightPerRestart verifies that a clean shutdown/restart cycle with no ticks
// leaves the running tick height at 0. Before the fix, the deferred final snapshot used the
// post-increment label and restore's unconditional +1 inflated the height by exactly 1 per restart.
func TestRestoreDriftsTickHeightPerRestart(t *testing.T) {
	w := newTestWorld(t)
	storage := w.snapshotStorage
	const zeroTickRestarts = 4
	for range zeroTickRestarts {
		w = newTestWorld(t)
		w.useSyncSnapshotStorage(storage)
		runSnapshotTicks(t, w, 0)
		require.NoError(t, w.restore(context.Background()))
	}
	assert.Equal(t, uint64(0), w.currentTick.height,
		"4 zero-tick restarts should leave height at 0, but it drifted to %d", w.currentTick.height)
}

// TestRestorePreservesTickHeightAfterTicks verifies that N real ticks followed by one clean restart
// keep the running tick height at N. Before the fix, the shifted to N+1 after a single restart.
func TestRestorePreservesTickHeightAfterTicks(t *testing.T) {
	w := newTestWorld(t)
	runSnapshotTicks(t, w, 5)
	require.NoError(t, w.restore(context.Background()))
	assert.Equal(t, uint64(5), w.currentTick.height,
		"after 5 ticks + one restart, height should still be 5, but it shifted to %d", w.currentTick.height)
}

// TestWriteFinalSnapshotSkipsWhenUnstarted verifies the deferred final snapshot writes nothing for a
// world that never ticked, so restore loads no snapshot and leaves the height at 0.
func TestWriteFinalSnapshotSkipsWhenUnstarted(t *testing.T) {
	w := newTestWorld(t)
	runSnapshotTicks(t, w, 0)
	_, err := w.snapshotStorage.Load(context.Background())
	assert.ErrorIs(t, err, snapshot.ErrSnapshotNotFound,
		"a world that never ticked must not write a final snapshot")
}

// TestWriteFinalSnapshotLabelsPreIncrement verifies the final snapshot is labeled with the
// pre-increment height (height-1), matching persistState's convention so restore's +1 is correct.
func TestWriteFinalSnapshotLabelsPreIncrement(t *testing.T) {
	w := newTestWorld(t)
	runSnapshotTicks(t, w, 5)

	data, err := w.snapshotStorage.Load(context.Background())
	require.NoError(t, err)
	snap, err := snapshot.Decode(data)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), snap.GetTickHeight(),
		"final snapshot after 5 ticks must use the pre-increment label (4), not the post-increment label (5)")
}

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
	w := newTestWorld(t) // SnapshotRate=1
	w.init()
	for range 3 {
		w.Tick(time.Now())
	}
	// After 3 ticks, height is 3. persistState wrote pre-increment labels 0, 1, 2 (last stored = 2).
	// Simulate a crash before the deferred final snapshot: do not shut down through run.
	require.NoError(t, w.restore(context.Background()))
	assert.Equal(
		t,
		uint64(3),
		w.currentTick.height,
		"a persistState (pre-increment=2) snapshot from tick 3 should restore to height 3, got %d",
		w.currentTick.height,
	)
}

// TestRepeatRestartHeightStable ticks the world three times, then repeats the clean restart cycle
// (run shutdown + restore, no new ticks) 4 times, asserting the height does not creep +1
// per restart when no further work is done — it must stay pinned at the last completed height.
func TestRepeatRestartHeightStable(t *testing.T) {
	w := newTestWorld(t)
	runSnapshotTicks(t, w, 3)
	storage := w.snapshotStorage
	const want uint64 = 3
	for r := range 4 {
		w = newTestWorld(t)
		w.useSyncSnapshotStorage(storage)
		runSnapshotTicks(t, w, 0)
		require.NoError(t, w.restore(context.Background()))
		assert.Equal(t, want, w.currentTick.height,
			"after restart %d (no new ticks), height should stay %d, got %d", r+1, want, w.currentTick.height)
	}
}
