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
// synchronous writer, so a test can drive the final-snapshot/restore half of a clean shutdown and
// restart cycle deterministically. The debug module is disabled; every debug hook the tick and
// restore paths touch is nil-safe.
func newTestWorld(t *testing.T) *World {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")

	debug := false
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

	// Run initialization systems so encodeSnapshot/Tick/restore operate on a live world.
	w.world.Init()
	return w
}

// TestRestoreDriftsTickHeightPerRestart verifies that a clean shutdown/restart cycle with no ticks
// leaves the running tick height at 0. Before the fix, the deferred final snapshot used the
// post-increment label and restore's unconditional +1 inflated the height by exactly 1 per restart.
func TestRestoreDriftsTickHeightPerRestart(t *testing.T) {
	w := newTestWorld(t)
	const zeroTickRestarts = 4
	for range zeroTickRestarts {
		w.writeFinalSnapshot()
		require.NoError(t, w.restore(context.Background()))
	}
	assert.Equal(t, uint64(0), w.currentTick.height,
		"4 zero-tick restarts should leave height at 0, but it drifted to %d", w.currentTick.height)
}

// TestRestorePreservesTickHeightAfterTicks verifies that N real ticks followed by one clean restart
// keep the running tick height at N. Before the fix, the shifted to N+1 after a single restart.
func TestRestorePreservesTickHeightAfterTicks(t *testing.T) {
	w := newTestWorld(t)
	for range 5 {
		w.Tick(time.Now())
	}
	w.writeFinalSnapshot()
	require.NoError(t, w.restore(context.Background()))
	assert.Equal(t, uint64(5), w.currentTick.height,
		"after 5 ticks + one restart, height should still be 5, but it shifted to %d", w.currentTick.height)
}

// TestWriteFinalSnapshotSkipsWhenUnstarted verifies the deferred final snapshot writes nothing for a
// world that never ticked, so restore loads no snapshot and leaves the height at 0.
func TestWriteFinalSnapshotSkipsWhenUnstarted(t *testing.T) {
	w := newTestWorld(t)
	w.writeFinalSnapshot()
	_, err := w.snapshotStorage.Load(context.Background())
	assert.ErrorIs(t, err, snapshot.ErrSnapshotNotFound,
		"a world that never ticked must not write a final snapshot")
}

// TestWriteFinalSnapshotLabelsPreIncrement verifies the final snapshot is labeled with the
// pre-increment height (height-1), matching persistState's convention so restore's +1 is correct.
func TestWriteFinalSnapshotLabelsPreIncrement(t *testing.T) {
	w := newTestWorld(t)
	for range 5 {
		w.Tick(time.Now())
	}
	w.writeFinalSnapshot()

	data, err := w.snapshotStorage.Load(context.Background())
	require.NoError(t, err)
	snap, err := snapshot.Decode(data)
	require.NoError(t, err)
	assert.Equal(t, uint64(4), snap.GetTickHeight(),
		"final snapshot after 5 ticks must use the pre-increment label (4), not the post-increment label (5)")
}
