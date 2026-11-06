package root

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// -------------------------------------------------------------------------------------------------
// shardPoolInstances
// -------------------------------------------------------------------------------------------------

// Replicas each keep their own state, so the expansion must list every pool
// instance — the same invariant the MCP reload's shardInstances enforces.
func TestShardPoolInstances_ExpandsToAllPoolInstances(t *testing.T) {
	t.Parallel()
	cfg := worldtoml.Config{Shards: []worldtoml.Shard{
		{ID: "game", InstanceID: "game"},
		{ID: "game", InstanceID: "game-2"},
		{ID: "lobby", InstanceID: "lobby"},
	}}
	assert.Equal(t, []string{"game", "game-2"}, shardPoolInstances(cfg, []string{"game"}))
	assert.Equal(t, []string{"lobby"}, shardPoolInstances(cfg, []string{"lobby"}))
}

func TestShardPoolInstances_MultipleShards(t *testing.T) {
	t.Parallel()
	cfg := worldtoml.Config{Shards: []worldtoml.Shard{
		{ID: "game", InstanceID: "game"},
		{ID: "game", InstanceID: "game-2"},
		{ID: "lobby", InstanceID: "lobby"},
		{ID: "chat", InstanceID: "chat"},
	}}
	assert.Equal(t, []string{"game", "game-2", "lobby"},
		shardPoolInstances(cfg, []string{"game", "lobby"}))
	assert.Equal(t, []string{"game", "game-2", "lobby", "chat"},
		shardPoolInstances(cfg, []string{"game", "lobby", "chat"}))
}

// An unexpanded shard (no InstanceID) falls back to its pool ID, matching the
// MCP shardInstances fallback.
func TestShardPoolInstances_FallsBackToShardID(t *testing.T) {
	t.Parallel()
	cfg := worldtoml.Config{Shards: []worldtoml.Shard{{ID: "game"}}}
	assert.Equal(t, []string{"game"}, shardPoolInstances(cfg, []string{"game"}))
}

func TestShardPoolInstances_UnknownShardYieldsNothing(t *testing.T) {
	t.Parallel()
	cfg := worldtoml.Config{Shards: []worldtoml.Shard{{ID: "game", InstanceID: "game"}}}
	assert.Empty(t, shardPoolInstances(cfg, []string{"nope"}))
}

func TestShardPoolInstances_EmptyShardIDs(t *testing.T) {
	t.Parallel()
	cfg := worldtoml.Config{Shards: []worldtoml.Shard{{ID: "game", InstanceID: "game"}}}
	assert.Empty(t, shardPoolInstances(cfg, nil))
}

// Instances are collected in world.toml order, so a pool's replicas appear in
// declaration order regardless of how many other shards are interleaved.
func TestShardPoolInstances_PreservesWorldTomlOrder(t *testing.T) {
	t.Parallel()
	cfg := worldtoml.Config{Shards: []worldtoml.Shard{
		{ID: "game", InstanceID: "game"},
		{ID: "lobby", InstanceID: "lobby"},
		{ID: "game", InstanceID: "game-2"},
	}}
	assert.Equal(t, []string{"game", "game-2"}, shardPoolInstances(cfg, []string{"game"}))
}

// -------------------------------------------------------------------------------------------------
// resolveReloadTargets
// -------------------------------------------------------------------------------------------------

// TestResolveReloadTargets_WaitInstanceIDsCoversAllPoolInstances is the core
// regression guard: naming one instance of a pool_size > 1 shard must produce a
// waitInstanceIDs list that covers every sibling, because the deploy recreates
// the whole pool. Before the fix, waitInstanceIDs was just the user-named
// instance, so a crashed sibling went unnoticed and the command exited 0.
func TestResolveReloadTargets_WaitInstanceIDsCoversAllPoolInstances(t *testing.T) {
	t.Parallel()
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Project: "demo",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},
			{ID: "game", InstanceID: "game-2"},
			{ID: "lobby", InstanceID: "lobby"},
		},
	}}
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
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Project: "demo",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},
			{ID: "game", InstanceID: "game-2"},
			{ID: "lobby", InstanceID: "lobby"},
		},
	}}
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
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Project: "demo",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},
			{ID: "game", InstanceID: "game-2"},
			{ID: "lobby", InstanceID: "lobby"},
		},
	}}
	targets, err := resolveReloadTargets(cfg, []string{"game-2"})
	require.NoError(t, err)

	shardSet := make(map[string]bool, len(targets.shardIDs))
	for _, id := range targets.shardIDs {
		shardSet[id] = true
	}
	instanceToShard := make(map[string]string, len(cfg.WorldToml.Shards))
	for _, sh := range cfg.WorldToml.Shards {
		instanceToShard[sh.InstanceID] = sh.ID
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
	for _, sh := range cfg.WorldToml.Shards {
		if !shardSet[sh.ID] {
			continue
		}
		assert.True(t, waitSet[sh.InstanceID],
			"shard %q instance %q is in a targeted pool but not in waitInstanceIDs",
			sh.ID, sh.InstanceID)
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
