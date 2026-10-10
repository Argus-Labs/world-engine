package root

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
)

// readyTick is one onProgress(ready, expected) invocation a fake
// WaitForShardsReady emits, in order.
type readyTick struct {
	ready, expected int
}

// fakeSession is a phasebox.Session that records every UpsertRow call so a
// test can assert the sequence of row transitions the readiness path emits.
type fakeSession struct {
	rows []fakeRow
}

type fakeRow struct {
	id, label, detail string
	state             phasebox.RowState
}

func (f *fakeSession) UpsertRow(id, label, detail string, state phasebox.RowState) {
	f.rows = append(f.rows, fakeRow{id, label, detail, state})
}

func (f *fakeSession) UpsertProgress(_ string, _ string, _ int) {}

// emitTicks returns a waitForReady closure that replays ticks as onProgress
// callbacks, mirroring how cli.WaitForShardsReady drives onProgress each poll.
func emitTicks(ticks []readyTick) func(onProgress func(ready, expected int)) {
	return func(onProgress func(ready, expected int)) {
		for _, tk := range ticks {
			onProgress(tk.ready, tk.expected)
		}
	}
}

// TestWaitForShardsReadyRow is the regression test for the readiness row never
// reaching a terminal state: every poll tick must upsert the "ready" row as
// Active, and the final upsert after the wait returns must transition it to
// Done (ready >= expected) or Failed ("timed out …") — never left Active.
func TestWaitForShardsReadyRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ticks      []readyTick
		wantFinal  phasebox.RowState
		wantDetail string
	}{
		{
			name:       "full success closes row Done with final N/M",
			ticks:      []readyTick{{0, 10}, {5, 10}, {10, 10}},
			wantFinal:  phasebox.Done,
			wantDetail: "10/10",
		},
		{
			name:       "single-shard success (default pool_size=1) closes Done",
			ticks:      []readyTick{{0, 1}, {1, 1}},
			wantFinal:  phasebox.Done,
			wantDetail: "1/1",
		},
		{
			name:       "timeout short of expected closes row Failed",
			ticks:      []readyTick{{0, 10}, {4, 10}, {8, 10}},
			wantFinal:  phasebox.Failed,
			wantDetail: "timed out (8/10 ready)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sess := &fakeSession{}
			waitForShardsReadyRow(sess, emitTicks(tt.ticks))

			require.NotEmpty(t, sess.rows, "at least the terminal upsert must run when onProgress fired")

			// Every tick produces one Active upsert; the terminal upsert is the
			// final row, so there are len(ticks)+1 rows total.
			require.Len(t, sess.rows, len(tt.ticks)+1, "one Active upsert per tick plus the terminal close")

			for i, row := range sess.rows[:len(tt.ticks)] {
				require.Equal(t, "ready", row.id, "tick row %d id", i)
				require.Equal(t, "Waiting for pods ready", row.label, "tick row %d label", i)
				require.Equal(t, phasebox.Active, row.state, "tick row %d must stay Active while the poll runs", i)
				require.Equal(t, fmt.Sprintf("%d/%d", tt.ticks[i].ready, tt.ticks[i].expected), row.detail,
					"tick row %d detail must reflect that tick's count", i)
			}

			last := sess.rows[len(sess.rows)-1]
			require.Equal(t, "ready", last.id, "terminal row id")
			require.Equal(t, "Waiting for pods ready", last.label, "terminal row label")
			require.Equal(t, tt.wantFinal, last.state, "terminal row state")
			require.Equal(t, tt.wantDetail, last.detail, "terminal row detail")
		})
	}
}

// TestWaitForShardsReadyRow_EarlySkipLeavesNoRow asserts the early-skip path:
// WaitForShardsReady's no-pools / no-clientset / no-desired-images branches
// return without a single onProgress tick, so no "ready" row was ever opened —
// waitForShardsReadyRow must not upsert a terminal row for one that doesn't
// exist (it would invent a spurious "ready" line under the deploy summary).
func TestWaitForShardsReadyRow_EarlySkipLeavesNoRow(t *testing.T) {
	t.Parallel()

	sess := &fakeSession{}
	waitForShardsReadyRow(sess, emitTicks(nil))

	require.Empty(t, sess.rows, "no onProgress tick means no ready row to close")
}
