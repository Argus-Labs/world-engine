package root

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// threeInstanceConfig is the shared two-pool fixture: a 2-instance "game" pool
// and a 1-instance "lobby" pool.
func threeInstanceConfig() *service.Config {
	return &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Project: "demo",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},
			{ID: "game", InstanceID: "game-2"},
			{ID: "lobby", InstanceID: "lobby"},
		},
	}}
}

// -------------------------------------------------------------------------------------------------
// resolveReloadTargets
// -------------------------------------------------------------------------------------------------

// TestResolveReloadTargets_WaitInstanceIDsCoversAllPoolInstances is the core
// regression guard: naming one instance of a pool_size > 1 shard must produce a
// waitInstanceIDs list that covers every sibling, because the deploy recreates
// the whole pool. Before the fix, the wait used instanceIDs (just the named
// instance), so a crashed sibling went unnoticed and the command exited 0.
func TestResolveReloadTargets_WaitInstanceIDsCoversAllPoolInstances(t *testing.T) {
	t.Parallel()
	cfg := threeInstanceConfig()
	targets, err := resolveReloadTargets(cfg, []string{"game-2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"game"}, targets.shardIDs, "shardIDs should be the pool ID")
	assert.Equal(t, []string{"game-2"}, targets.instanceIDs,
		"instanceIDs should be the user-named instance only")
	assert.Equal(t, []string{"game", "game-2"}, targets.poolInstances(targets.shardIDs),
		"waitInstanceIDs must cover every sibling of the deployed pool, not just the named instance")
}

// Empty instanceIDs (the default reload and `world start`) waits on every
// instance — the same behavior as before the fix, since shardIDs is every pool.
func TestResolveReloadTargets_EmptyInstanceIDsWaitsAll(t *testing.T) {
	t.Parallel()
	cfg := threeInstanceConfig()
	targets, err := resolveReloadTargets(cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"game", "game-2", "lobby"}, targets.poolInstances(targets.shardIDs),
		"empty instanceIDs (default reload) should wait on every instance")
}

// Naming instances from multiple pools must expand each pool to all its
// instances, not just the named ones.
func TestResolveReloadTargets_MultiplePoolsWaitAll(t *testing.T) {
	t.Parallel()
	cfg := threeInstanceConfig()
	cfg.WorldToml.Shards = append(cfg.WorldToml.Shards, worldtoml.Shard{ID: "lobby", InstanceID: "lobby-2"})
	targets, err := resolveReloadTargets(cfg, []string{"game-2", "lobby"})
	require.NoError(t, err)
	assert.Equal(t, []string{"game", "lobby"}, targets.shardIDs)
	assert.Equal(t, []string{"game-2", "lobby"}, targets.instanceIDs)
	assert.Equal(t, []string{"game", "game-2", "lobby", "lobby-2"}, targets.poolInstances(targets.shardIDs),
		"waitInstanceIDs must cover every instance of both deployed pools")
}

// Naming every instance of a pool explicitly must produce the same
// waitInstanceIDs as naming just one — the expansion is idempotent.
func TestResolveReloadTargets_NamingAllInstancesMatchesNamingOne(t *testing.T) {
	t.Parallel()
	cfg := threeInstanceConfig()
	one, err := resolveReloadTargets(cfg, []string{"game-2"})
	require.NoError(t, err)
	all, err := resolveReloadTargets(cfg, []string{"game", "game-2"})
	require.NoError(t, err)
	assert.Equal(t, one.poolInstances(one.shardIDs), all.poolInstances(all.shardIDs),
		"naming one instance or all must produce the same wait scope (the whole pool)")
}

// -------------------------------------------------------------------------------------------------
// rollShards
// -------------------------------------------------------------------------------------------------

// fakeShardRoller is a shardRoller stand-in for unit-testing rollShards without
// a Docker daemon. It records the shard IDs handed to Deploy and the instance IDs
// handed to WaitForShardsReady, and returns caller-configured ready/expected
// counts, so a test can assert that the wait scope matches the deploy scope.
type fakeShardRoller struct {
	gotDeploy    []string
	gotWait      []string
	waitReady    int
	waitExpected int
}

func (*fakeShardRoller) PruneOrphanedShards(context.Context) error { return nil }
func (f *fakeShardRoller) Deploy(_ context.Context, opts local.DeployOpts) error {
	f.gotDeploy = append(f.gotDeploy, opts.ShardIDs()...)
	return nil
}
func (f *fakeShardRoller) WaitForShardsReady(_ context.Context, instanceIDs []string, _ func(int, int)) (int, int) {
	f.gotWait = instanceIDs
	return f.waitReady, f.waitExpected
}

// TestRollShards is the rollShards-level regression guard: the wait scope must
// match the deploy scope — Deploy recreates every instance of each targeted
// pool, so rollShards must wait on every sibling or a crashed one is missed.
// Both cases share the fixture, dashboard, invocation, and wait-scope
// assertions; only the readiness counts and the expected error differ. The
// WaitScopeCoversWholePool case has game down + game-2 healthy, surfacing as
// shardsNotReadyError (1/2 ready) with the unrelated lobby pool out of scope.
// The AllReadyReturnsNil case is the happy path: every waited instance comes
// back, so rollShards returns nil.
func TestRollShards(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		waitReady    int
		waitExpected int
		wantNotReady bool
	}{
		{name: "WaitScopeCoversWholePool", waitReady: 1, waitExpected: 2, wantNotReady: true},
		{name: "AllReadyReturnsNil", waitReady: 2, waitExpected: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			dash := phasebox.Start(ctx, phasebox.Plain)
			defer dash.Complete()

			cfg := threeInstanceConfig()
			// Resolve both pools but deploy only game: the resolved shard set is
			// {game, lobby}, so a future change that derives the wait from
			// targets.shardIDs would wait on lobby too. The literal wait-list
			// assertion below catches that drift.
			targets, err := resolveReloadTargets(cfg, []string{"game-2", "lobby"})
			require.NoError(t, err)

			fake := &fakeShardRoller{waitReady: tt.waitReady, waitExpected: tt.waitExpected}
			err = dash.Run("Shards",
				func(ctx context.Context, sess phasebox.Session) error {
					return rollShards(ctx, sess, fake, targets, []local.DeployShard{{ID: "game"}}, true)
				},
				func(time.Duration) string { return "reloaded" },
			)
			if tt.wantNotReady {
				require.Error(t, err)
				var notReady *shardsNotReadyError
				require.ErrorAs(t, err, &notReady)
				assert.Equal(t, tt.waitReady, notReady.ready)
				assert.Equal(t, tt.waitExpected, notReady.expected)
			} else {
				require.NoError(t, err)
			}
			// The wait covered the whole game pool; the unrelated resolved lobby pool stayed out.
			// This literal assertion is what catches a switch back to targets.shardIDs, since
			// the resolved shard set is {game, lobby} while the deployed set is {game}.
			assert.Equal(t, []string{"game", "game-2"}, fake.gotWait)
			// The deploy scope is the named game pool only; the resolved lobby
			// pool (in targets.shardIDs) must not be deployed.
			assert.Equal(t, []string{"game"}, fake.gotDeploy)
		})
	}
}

// -------------------------------------------------------------------------------------------------
// waitForShardsReadyRow / waitForShardsReadyResult
// -------------------------------------------------------------------------------------------------

// fakeSession records the last upserted row so tests can assert the
// readiness row's terminal state without a live bubbletea program. Fail also
// records the error passed at that boundary so tests can assert the cancel
// classification directly; the "canceled" label rendering is phasebox's job
// and is covered there, not duplicated here.
type fakeSession struct {
	last    fakeRow
	seen    bool
	failID  string
	failErr error
}

type fakeRow struct {
	id, detail string
	state      phasebox.RowState
}

func (f *fakeSession) UpsertRow(id, _, detail string, state phasebox.RowState) {
	f.last = fakeRow{id: id, detail: detail, state: state}
	f.seen = true
}

func (*fakeSession) UpsertProgress(string, string, int) {}

func (f *fakeSession) Fail(id, label string, err error) {
	f.failID = id
	f.failErr = err
	f.UpsertRow(id, label, "failed", phasebox.Failed)
}

// lastRow returns the last upserted row, failing the test if none was recorded.
func (f *fakeSession) lastRow(t *testing.T) fakeRow {
	t.Helper()
	if !f.seen {
		t.Fatal("no rows upserted")
	}
	return f.last
}

// fakeReadinessWaiter is a stand-in shardReadinessWaiter. It mimics the real
// WaitForShardsReady: on a canceled ctx it returns (0, expected) without
// calling onProgress (the cancel-before-first-tick case); otherwise it calls
// onProgress once with (ready, expected) and returns them. onProgressHook,
// if set, fires after onProgress so a test can cancel from the callback.
type fakeReadinessWaiter struct {
	ready, expected int
	onProgressHook   func()
}

func (f *fakeReadinessWaiter) WaitForShardsReady(
	ctx context.Context, _ []string, onProgress func(ready, expected int),
) (int, int) {
	if ctx.Err() != nil {
		return 0, f.expected
	}
	onProgress(f.ready, f.expected)
	if f.onProgressHook != nil {
		f.onProgressHook()
	}
	return f.ready, f.expected
}

// --- waitForShardsReadyRow ---------------------------------------------------

// Ctrl+C during the wait routes through sess.Fail (not the timed-out
// UpsertRow): the timeout never elapsed, the user did. The canceled label
// rendering is phasebox's job and is covered there, not duplicated here.
func TestWaitForShardsReadyRow_CanceledRoutesThroughFail(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sess := &fakeSession{}
	waitForShardsReadyRow(ctx, sess, func(onProgress func(ready, expected int)) {
		onProgress(0, 2)
	})

	row := sess.lastRow(t)
	assert.Equal(t, "ready", row.id)
	assert.Equal(t, phasebox.Failed, row.state)
	assert.Equal(t, "ready", sess.failID, "a cancel routes through Fail, not the timed-out UpsertRow")
	assert.ErrorIs(t, sess.failErr, context.Canceled, "a cancel must not be labeled 'timed out'")
}

// A genuine timeout — ctx alive with lastReady < expected, or a caller
// deadline that already expired — both show "timed out"; only [context.Canceled]
// is a cancel, DeadlineExceeded is a real timeout.
func TestWaitForShardsReadyRow_TimeoutShowsTimedOut(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ctx  func() context.Context
	}{
		{"liveContext", context.Background},
		{"expiredDeadline", func() context.Context {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
			t.Cleanup(cancel)
			return ctx
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sess := &fakeSession{}
			waitForShardsReadyRow(c.ctx(), sess, func(onProgress func(ready, expected int)) {
				onProgress(0, 2)
			})

			row := sess.lastRow(t)
			assert.Equal(t, phasebox.Failed, row.state)
			assert.Contains(t, row.detail, "timed out")
			assert.Contains(t, row.detail, "0/2 ready")
		})
	}
}

// All shards ready shows Done with the final count.
func TestWaitForShardsReadyRow_ReadyShowsDone(t *testing.T) {
	t.Parallel()

	sess := &fakeSession{}
	waitForShardsReadyRow(context.Background(), sess, func(onProgress func(ready, expected int)) {
		onProgress(2, 2)
	})

	row := sess.lastRow(t)
	assert.Equal(t, phasebox.Done, row.state)
	assert.Equal(t, "2/2", row.detail)
}

// If the wait returned without ever invoking onProgress (cancel before the
// first completed probe round), no terminal row is rendered — the row stays
// absent rather than rendering a terminal claim with no evidence.
func TestWaitForShardsReadyRow_NoProgressNoTerminalRow(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sess := &fakeSession{}
	waitForShardsReadyRow(ctx, sess, func(func(ready, expected int) {
		// Cancel before first tick: onProgress is never called.
	})

	assert.False(t, sess.seen, "no progress tick means no terminal row")
}

// --- waitForShardsReadyResult -----------------------------------------------

// Ctrl+C before the first progress tick is a cancel, not a shards-not-ready
// failure: the helper passes [context.Canceled] through for phasebox to
// silence at the section boundary.
func TestWaitForShardsReadyResult_CanceledBeforeProgressReturnsCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	waiter := &fakeReadinessWaiter{expected: 2}
	err := waitForShardsReadyResult(ctx, &fakeSession{}, waiter, nil)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err, "cancel must pass context.Canceled through")
	var notReady *shardsNotReadyError
	assert.False(t, errors.As(err, &notReady), "must not be a shardsNotReadyError")
}

// Ctrl+C arriving during the wait — after a partial or complete count was
// reported — is still a cancel: cancellation wins even when the counts are
// complete. The row is marked canceled through sess.Fail before the section
// ends, and Box.Run supplies the canceled summary and silent wrapper at the
// boundary. The helper returns [context.Canceled] itself (not wrapped).
func TestWaitForShardsReadyResult_CanceledAfterProgressReturnsCanceled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		ready int
	}{
		{"AfterPartialProgress", 1},
		{"AfterComplete", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sess := &fakeSession{}
			err := waitForShardsReadyResult(ctx, sess, &fakeReadinessWaiter{
				ready: c.ready, expected: 2,
				onProgressHook: cancel,
			}, nil)

			require.Error(t, err)
			assert.Equal(t, context.Canceled, err)
			assert.Equal(t, "ready", sess.failID, "cancel routes through Fail")
			assert.ErrorIs(t, sess.failErr, context.Canceled, "Fail must receive context.Canceled")
		})
	}
}

// A genuine timeout (ctx alive, ready < expected) returns shardsNotReadyError
// — no regression in the real-failure path.
func TestWaitForShardsReadyResult_TimeoutReturnsShardsNotReadyError(t *testing.T) {
	t.Parallel()

	waiter := &fakeReadinessWaiter{expected: 2}
	err := waitForShardsReadyResult(context.Background(), &fakeSession{}, waiter, nil)

	require.Error(t, err)
	var notReady *shardsNotReadyError
	require.ErrorAs(t, err, &notReady)
	assert.Equal(t, 0, notReady.ready)
	assert.Equal(t, 2, notReady.expected)
}

// All shards ready returns nil — no regression in the success path.
func TestWaitForShardsReadyResult_ReadyReturnsNil(t *testing.T) {
	t.Parallel()

	waiter := &fakeReadinessWaiter{ready: 2, expected: 2}
	err := waitForShardsReadyResult(context.Background(), &fakeSession{}, waiter, nil)

	assert.NoError(t, err)
}

// The cancel survives JoinFailures packaging (deployShardImages wraps the
// shards error through JoinFailures with the purge error): a cancel alone
// must not print.
func TestWaitForShardsReadyResult_CancelSurvivesJoinFailures(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	waiter := &fakeReadinessWaiter{expected: 2}
	shardsErr := waitForShardsReadyResult(ctx, &fakeSession{}, waiter, nil)

	onlyCancel := errorspkg.JoinFailures(nil, shardsErr)
	require.Error(t, onlyCancel)
	assert.False(t, errorspkg.ShouldPrint(onlyCancel), "only a cancel: must not print")
}

// Cancellation through a real phasebox section (Plain progress, the CI/log
// surface) classifies as "canceled", not a shard failure: Box.Run silences
// [context.Canceled] and the summary omits the shard-failure diagnostic.
func TestWaitForShardsReadyResult_CancelThroughPlainSectionIsCanceled(t *testing.T) {
	// Not parallel: phasebox.Start captures os.Stdout for Plain progress, so
	// the redirect below mutates a global briefly.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close() // closing the writer and reading EOF leaves the reader descriptor open otherwise.
	os.Stdout = w   //nolint:reassign // capturing stdout in tests
	dash := phasebox.Start(ctx, phasebox.Plain)
	os.Stdout = orig //nolint:reassign // restore stdout
	defer dash.Complete()
	var out bytes.Buffer
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&out, r)
		close(copyDone)
	}()

	runErr := dash.Run("Shards",
		func(ctx context.Context, sess phasebox.Session) error {
			return waitForShardsReadyResult(ctx, sess, &fakeReadinessWaiter{expected: 2}, nil)
		},
		func(time.Duration) string { return "done" },
	)
	require.NoError(t, w.Close())
	<-copyDone

	require.Error(t, runErr)
	assert.True(t, errorspkg.IsSilent(runErr), "cancel through section must be silent, not a shard failure")
	assert.ErrorIs(t, runErr, context.Canceled, "section must classify cancel as canceled")
	var notReady *shardsNotReadyError
	assert.False(t, errors.As(runErr, &notReady), "must not be a shardsNotReadyError")
	assert.Contains(t, out.String(), "canceled", "plain summary must say canceled")
}

// A caller deadline ([context.DeadlineExceeded], not Ctrl+C) is a real
// failure, not a cancel: waitForShardsReadyResult classifies it as a
// shardsNotReadyError so the failure is reported, not hidden — only
// [context.Canceled] is silent.
func TestWaitForShardsReadyResult_DeadlineExceededIsNotSilent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	waiter := &fakeReadinessWaiter{expected: 2}
	err := waitForShardsReadyResult(ctx, &fakeSession{}, waiter, nil)

	require.Error(t, err)
	var notReady *shardsNotReadyError
	require.ErrorAs(t, err, &notReady, "deadline with unready shards is a real failure")
	assert.True(t, errorspkg.ShouldPrint(err), "deadline with unready shards must be printable, not silent")
}
