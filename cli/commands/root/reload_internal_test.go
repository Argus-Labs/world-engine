package root

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
)

// fakeSession records every UpsertRow call so tests can assert on the exact
// sequence of row states phasebox renders. Implements phasebox.Session.
type fakeSession struct {
	rows  []fakeRow
	fails []fakeFail
}

type fakeRow struct {
	id     string
	label  string
	detail string
	state  phasebox.RowState
}

type fakeFail struct {
	id    string
	label string
	err   error
}

func (s *fakeSession) UpsertRow(id, label, detail string, state phasebox.RowState) {
	s.rows = append(s.rows, fakeRow{id: id, label: label, detail: detail, state: state})
}

func (s *fakeSession) UpsertProgress(id, label string, percent int) {}

func (s *fakeSession) Fail(id, label string, err error) {
	s.fails = append(s.fails, fakeFail{id: id, label: label, err: err})
}

// rowsFor returns every UpsertRow call for the given id, in insertion order.
func (s *fakeSession) rowsFor(id string) []fakeRow {
	var out []fakeRow
	for _, r := range s.rows {
		if r.id == id {
			out = append(out, r)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// shouldWaitForShardsReady — the gating fix
// ---------------------------------------------------------------------------

func TestShouldWaitForShardsReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		purgeErr error
		want     bool
	}{
		{
			name:     "no purge error waits for readiness (world start / plain reload)",
			purgeErr: nil,
			want:     true,
		},
		{
			name:     "successful purge still waits for readiness",
			purgeErr: nil,
			want:     true,
		},
		{
			name:     "failed purge skips the wait",
			purgeErr: errors.New("purge failed"),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, shouldWaitForShardsReady(tt.purgeErr))
		})
	}
}

// Regression: the old gate `purge && purgeErr == nil` returned false for every
// non-purge path. The fix `purgeErr == nil` returns true there, so world start
// and plain world reload now wait for shard pods to be Ready.
func TestShouldWaitForShardsReady_NonPurgePathWaits(t *testing.T) {
	t.Parallel()
	// world start: purge=false, purgeErr=nil.
	// Old code: false && true == false  (skip wait — BUG)
	// New code:                  true    (wait     — FIX)
	require.True(t, shouldWaitForShardsReady(nil),
		"world start (no purge) must wait for shard readiness")
}

// ---------------------------------------------------------------------------
// waitForShardsReadyRow — the readiness row behavior that was being skipped
// ---------------------------------------------------------------------------

// All pods Ready by the deadline → terminal Done row with "N/N".
func TestWaitForShardsReadyRow_AllReady(t *testing.T) {
	t.Parallel()
	sess := &fakeSession{}
	waitForShardsReadyRow(sess, func(onProgress func(ready, expected int)) {
		onProgress(0, 2)
		onProgress(1, 2)
		onProgress(2, 2)
	})

	readyRows := sess.rowsFor("ready")
	require.Len(t, readyRows, 4, "3 progress ticks + 1 terminal update")

	for i := range 3 {
		require.Equal(t, phasebox.Active, readyRows[i].state, "tick %d should be Active", i)
		require.Equal(t, "Waiting for pods ready", readyRows[i].label)
	}

	last := readyRows[3]
	require.Equal(t, phasebox.Done, last.state)
	require.Equal(t, "2/2", last.detail)
}

// Not all pods Ready by the deadline → terminal Failed row with "timed out".
func TestWaitForShardsReadyRow_TimedOut(t *testing.T) {
	t.Parallel()
	sess := &fakeSession{}
	waitForShardsReadyRow(sess, func(onProgress func(ready, expected int)) {
		onProgress(0, 3)
		onProgress(1, 3)
		onProgress(1, 3)
	})

	readyRows := sess.rowsFor("ready")
	require.Len(t, readyRows, 4)

	last := readyRows[3]
	require.Equal(t, phasebox.Failed, last.state)
	require.Equal(t, "timed out (1/3 ready)", last.detail)
}

// The early-skip path (WaitForShardsReady returned without a single tick —
// no pools, no clientset, no desired images) leaves no terminal row.
func TestWaitForShardsReadyRow_EarlySkipNoProgress(t *testing.T) {
	t.Parallel()
	sess := &fakeSession{}
	waitForShardsReadyRow(sess, func(onProgress func(ready, expected int)) {
		// Simulate WaitForShardsReady's early-return paths: onProgress
		// is never invoked.
	})

	require.Empty(t, sess.rowsFor("ready"), "no progress ticks means no ready row at all")
}

// Progress detail string format: "N/M" on every tick.
func TestWaitForShardsReadyRow_ProgressFormat(t *testing.T) {
	t.Parallel()
	sess := &fakeSession{}
	waitForShardsReadyRow(sess, func(onProgress func(ready, expected int)) {
		onProgress(0, 4)
		onProgress(2, 4)
		onProgress(4, 4)
	})

	readyRows := sess.rowsFor("ready")
	require.Equal(t, "0/4", readyRows[0].detail)
	require.Equal(t, "2/4", readyRows[1].detail)
	require.Equal(t, "4/4", readyRows[2].detail)
	require.Equal(t, "4/4", readyRows[3].detail)
}

// Single tick with all Ready → Done immediately.
func TestWaitForShardsReadyRow_SingleTickAllReady(t *testing.T) {
	t.Parallel()
	sess := &fakeSession{}
	waitForShardsReadyRow(sess, func(onProgress func(ready, expected int)) {
		onProgress(1, 1)
	})

	readyRows := sess.rowsFor("ready")
	require.Len(t, readyRows, 2)

	require.Equal(t, phasebox.Active, readyRows[0].state)
	require.Equal(t, "1/1", readyRows[0].detail)

	require.Equal(t, phasebox.Done, readyRows[1].state)
	require.Equal(t, "1/1", readyRows[1].detail)
}

// A partial-ready final tick produces a Failed terminal row, not Done.
func TestWaitForShardsReadyRow_PartialReadyOnLastTick(t *testing.T) {
	t.Parallel()
	sess := &fakeSession{}
	waitForShardsReadyRow(sess, func(onProgress func(ready, expected int)) {
		onProgress(1, 2)
	})

	readyRows := sess.rowsFor("ready")
	require.Len(t, readyRows, 2)
	require.Equal(t, phasebox.Active, readyRows[0].state)
	require.Equal(t, phasebox.Failed, readyRows[1].state)
	require.Equal(t, "timed out (1/2 ready)", readyRows[1].detail)
}
