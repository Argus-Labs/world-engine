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
	assert.Equal(t, []string{"game", "game-2"}, targets.waitInstanceIDs,
		"waitInstanceIDs must cover every sibling of the deployed pool, not just the named instance")
}

// Empty instanceIDs (the default reload and `world start`) waits on every
// instance — the same behavior as before the fix, since shardIDs is every pool.
func TestResolveReloadTargets_EmptyInstanceIDsWaitsAll(t *testing.T) {
	t.Parallel()
	cfg := threeInstanceConfig()
	targets, err := resolveReloadTargets(cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"game", "game-2", "lobby"}, targets.waitInstanceIDs,
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
	assert.Equal(t, []string{"game", "game-2", "lobby", "lobby-2"}, targets.waitInstanceIDs,
		"waitInstanceIDs must cover every instance of both deployed pools")
}

// TestResolveReloadTargets_WaitScopeMatchesDeployScope verifies the key
// invariant the fix establishes: every instance in waitInstanceIDs belongs to a
// shard in shardIDs, and every instance of those shards in world.toml is in
// waitInstanceIDs. This is what prevents a crashed sibling from going
// unnoticed — the wait scope is exactly the deploy scope.
func TestResolveReloadTargets_WaitScopeMatchesDeployScope(t *testing.T) {
	t.Parallel()
	cfg := threeInstanceConfig()
	targets, err := resolveReloadTargets(cfg, []string{"game-2"})
	require.NoError(t, err)

	shardSet := make(map[string]bool, len(targets.shardIDs))
	for _, id := range targets.shardIDs {
		shardSet[id] = true
	}
	instanceToShard := make(map[string]string, len(cfg.WorldToml.Shards))
	for i := range cfg.WorldToml.Shards {
		s := &cfg.WorldToml.Shards[i]
		instanceToShard[s.InstanceID] = s.ID
	}

	// Every wait instance belongs to a targeted shard.
	for _, instID := range targets.waitInstanceIDs {
		shardID := instanceToShard[instID]
		assert.True(t, shardSet[shardID],
			"wait instance %q maps to shard %q which is not in shardIDs", instID, shardID)
	}

	// Every instance of a targeted shard is in the wait list.
	waitSet := make(map[string]bool, len(targets.waitInstanceIDs))
	for _, id := range targets.waitInstanceIDs {
		waitSet[id] = true
	}
	for i := range cfg.WorldToml.Shards {
		s := &cfg.WorldToml.Shards[i]
		if !shardSet[s.ID] {
			continue
		}
		assert.True(t, waitSet[s.InstanceID],
			"shard %q instance %q is in a targeted pool but not in waitInstanceIDs",
			s.ID, s.InstanceID)
	}
}

// Naming every instance of a pool explicitly must produce the same
// waitInstanceIDs as naming just one — the expansion is idempotent.
func TestResolveReloadTargets_NamingAllInstancesMatchesNamingOne(t *testing.T) {
	t.Parallel()
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Project: "demo",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},
			{ID: "game", InstanceID: "game-2"},
		},
	}}
	one, err := resolveReloadTargets(cfg, []string{"game-2"})
	require.NoError(t, err)
	all, err := resolveReloadTargets(cfg, []string{"game", "game-2"})
	require.NoError(t, err)
	assert.Equal(t, one.waitInstanceIDs, all.waitInstanceIDs,
		"naming one instance or all must produce the same wait scope (the whole pool)")
}

// -------------------------------------------------------------------------------------------------
// rollShards
// -------------------------------------------------------------------------------------------------

// fakeShardRoller is a shardRoller stand-in for unit-testing rollShards without
// a Docker daemon. It records the instance IDs handed to WaitForShardsReady and
// returns caller-configured ready/expected counts, so a test can assert the wait
// scope directly.
type fakeShardRoller struct {
	gotWait      []string
	waitReady    int
	waitExpected int
}

func (f *fakeShardRoller) PruneOrphanedShards(context.Context) error { return nil }
func (f *fakeShardRoller) Deploy(context.Context, local.DeployOpts) error {
	return nil
}
func (f *fakeShardRoller) WaitForShardsReady(_ context.Context, instanceIDs []string, _ func(int, int)) (int, int) {
	f.gotWait = instanceIDs
	return f.waitReady, f.waitExpected
}

// TestRollShards_WaitScopeCoversWholePool is the rollShards-level regression
// guard: the wait scope must match the deploy scope — Deploy recreates every
// instance of each targeted pool, so rollShards must wait on every sibling or a
// crashed one is missed. Here game down + game-2 healthy surfaces as
// shardsNotReadyError (1/2 ready); the unrelated lobby pool stays out of scope.
func TestRollShards_WaitScopeCoversWholePool(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dash := phasebox.Start(ctx, phasebox.Plain)
	defer dash.Complete()

	cfg := threeInstanceConfig()
	targets, err := resolveReloadTargets(cfg, []string{"game-2"})
	require.NoError(t, err)
	require.Equal(t, []string{"game"}, targets.shardIDs)
	require.Equal(t, []string{"game-2"}, targets.instanceIDs)
	require.Equal(t, []string{"game", "game-2"}, targets.waitInstanceIDs)

	fake := &fakeShardRoller{waitReady: 1, waitExpected: 2} // game down, game-2 healthy
	err = dash.Run("Shards",
		func(ctx context.Context, sess phasebox.Session) error {
			return rollShards(ctx, sess, fake, targets, []local.DeployShard{{ID: "game"}}, true)
		},
		func(time.Duration) string { return "reloaded" },
	)
	require.Error(t, err)
	var notReady *shardsNotReadyError
	require.ErrorAs(t, err, &notReady)
	assert.Equal(t, 1, notReady.ready)
	assert.Equal(t, 2, notReady.expected)
	// The wait covered the whole game pool; the unrelated lobby pool stayed out.
	assert.Equal(t, []string{"game", "game-2"}, fake.gotWait)
}

// TestRollShards_AllReadyExitsZero covers the happy path: when every waited
// instance comes back, rollShards returns nil.
func TestRollShards_AllReadyExitsZero(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dash := phasebox.Start(ctx, phasebox.Plain)
	defer dash.Complete()

	cfg := threeInstanceConfig()
	targets, err := resolveReloadTargets(cfg, []string{"game-2"})
	require.NoError(t, err)

	fake := &fakeShardRoller{waitReady: 2, waitExpected: 2} // both game instances healthy
	err = dash.Run("Shards",
		func(ctx context.Context, sess phasebox.Session) error {
			return rollShards(ctx, sess, fake, targets, []local.DeployShard{{ID: "game"}}, true)
		},
		func(time.Duration) string { return "reloaded" },
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"game", "game-2"}, fake.gotWait)
}
