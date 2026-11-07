package root

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// threeInstanceConfig is the shared two-pool fixture: a 2-instance "game" pool
// and a 1-instance "lobby" pool. Tests that need a different layout build their
// own Config; the expected instance lists stay explicit in each test.
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
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Project: "demo",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},
			{ID: "game", InstanceID: "game-2"},
			{ID: "lobby", InstanceID: "lobby"},
			{ID: "lobby", InstanceID: "lobby-2"},
		},
	}}
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
	for _, s := range opts.Shards {
		f.gotDeploy = append(f.gotDeploy, s.ID)
	}
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
			targets, err := resolveReloadTargets(cfg, []string{"game-2"})
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
			// The wait covered the whole game pool; the unrelated lobby pool stayed out.
			assert.Equal(t, []string{"game", "game-2"}, fake.gotWait)
			// The wait scope is exactly the pool instances of the deployed shard IDs, so a
			// future change that drops the deploy-scope forwarding would fail here too.
			assert.Equal(t, cfg.WorldToml.PoolInstances(fake.gotDeploy), fake.gotWait)
		})
	}
}
