package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// Sentinel errors injected into the fake so assertions can confirm a wipe failure
// survives the errors.Join + eris.Wrap plumbing and is surfaced to the caller
// (rather than swallowed by a later success or masked by a deploy error).
var (
	errPurgeBoom  = errors.New("purge boom")
	errDeployBoom = errors.New("deploy boom")
)

// fakeReloadClient is a hand-rolled stand-in for *cluster.Client that records
// every invocation (in order) and returns injected per-shard / per-instance
// errors. It exists only so reloadShards' purge-loop resilience contract can
// be exercised without a live k3d cluster; *cluster.Client is the sole
// production implementation and satisfies reloadClient unmodified.
type fakeReloadClient struct {
	callLog []string // every method invocation in order, for ordering asserts.

	// Injected errors, keyed by shard or instance ID. A missing key is nil.
	undeployErrs    map[string]error
	purgeErrs       map[string]error
	deployShardErrs map[string]error
	pruneErr        error
	deployErr       error

	// Recorded arguments for behavioral asserts.
	undeployed     []string
	purgeArgs      []purgeCall
	redeployed     []string
	deployOptsSeen []cluster.DeployOpts
	waitReadyCalls int
}

type purgeCall struct {
	org, project, instanceID string
}

func newFakeReloadClient() *fakeReloadClient {
	return &fakeReloadClient{
		undeployErrs:    make(map[string]error),
		purgeErrs:       make(map[string]error),
		deployShardErrs: make(map[string]error),
	}
}

func (f *fakeReloadClient) UndeployShard(_ context.Context, shardID string) error {
	f.callLog = append(f.callLog, "undeploy:"+shardID)
	f.undeployed = append(f.undeployed, shardID)
	return f.undeployErrs[shardID]
}

func (f *fakeReloadClient) PurgeShardState(_ context.Context, org, project, instanceID string) error {
	f.callLog = append(f.callLog, "purge:"+instanceID)
	f.purgeArgs = append(f.purgeArgs, purgeCall{org: org, project: project, instanceID: instanceID})
	return f.purgeErrs[instanceID]
}

func (f *fakeReloadClient) DeployShard(_ context.Context, _ worldtoml.Config, shardID string) error {
	f.callLog = append(f.callLog, "deployShard:"+shardID)
	f.redeployed = append(f.redeployed, shardID)
	return f.deployShardErrs[shardID]
}

func (f *fakeReloadClient) PruneOrphanedShards(_ context.Context, _ worldtoml.Config) error {
	f.callLog = append(f.callLog, "prune")
	return f.pruneErr
}

func (f *fakeReloadClient) Deploy(_ context.Context, opts cluster.DeployOpts) error {
	f.callLog = append(f.callLog, "deploy")
	f.deployOptsSeen = append(f.deployOptsSeen, opts)
	return f.deployErr
}

func (f *fakeReloadClient) WaitForShardsReady(_ context.Context, _ worldtoml.Config, _ func(ready, expected int)) {
	f.callLog = append(f.callLog, "waitReady")
	f.waitReadyCalls++
}

// reloadTestConfig is a two-shard world (game with two replicas + lobby) used
// to exercise per-instance purge granularity and multi-shard continuation.
func reloadTestConfig() worldtoml.Config {
	return worldtoml.Config{
		Organization: "argus",
		Project:      "rampage",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},
			{ID: "game", InstanceID: "game-2"},
			{ID: "lobby", InstanceID: "lobby"},
		},
	}
}

// reloadTestDeployOpts is the operator Deploy payload reloadShards rolls: one
// entry per logical shard ID (pool replicas deduped), matching buildWorldShards.
func reloadTestDeployOpts() cluster.DeployOpts {
	return cluster.DeployOpts{
		Project: "rampage",
		Shards: []cluster.DeployShard{
			{ID: "game", SourceImage: "argus-rampage-game-shard:latest"},
			{ID: "lobby", SourceImage: "argus-rampage-lobby-shard:latest"},
		},
	}
}

// instances returns the instanceIDs the fake saw PurgeShardState called with,
// in order, for asserting per-replica wipe granularity.
func instances(calls []purgeCall) []string {
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.instanceID)
	}
	return out
}

// shardIDs returns the shard IDs in a []DeployShard, in order.
func shardIDs(shards []cluster.DeployShard) []string {
	out := make([]string, 0, len(shards))
	for _, s := range shards {
		out = append(out, s.ID)
	}
	return out
}

// happyPathCallLog is the exact call sequence reloadShards performs on a clean
// purge of game (two replicas) then lobby, followed by the always-on image roll.
var happyPathCallLog = []string{
	"undeploy:game", "purge:game", "purge:game-2", "deployShard:game",
	"undeploy:lobby", "purge:lobby", "deployShard:lobby",
	"prune", "deploy", "waitReady",
}

// The happy path pins the baseline purge ordering: per shard, undeploy -> wipe
// every instance -> re-apply the CR; then the always-on prune -> deploy -> wait.
func TestReloadShards_PurgeHappyPath_OrderAndNilError(t *testing.T) {
	t.Parallel()
	fake := newFakeReloadClient()

	err := reloadShards(context.Background(), fake, reloadTestConfig(), reloadTestDeployOpts(), true)

	require.NoError(t, err)
	assert.Equal(t, happyPathCallLog, fake.callLog)
	// Per-instance granularity: game's two replicas are wiped independently.
	assert.Equal(t, []string{"game", "game-2", "lobby"}, instances(fake.purgeArgs))
	require.Len(t, fake.deployOptsSeen, 1)
	assert.Equal(t, []string{"game", "lobby"}, shardIDs(fake.deployOptsSeen[0].Shards))
	assert.Equal(t, 1, fake.waitReadyCalls)
}

// Without purge, the purge loop is skipped entirely and the flow goes straight
// to the trailing image roll, so a reload that doesn't wipe is never entangled
// with the purge-only error-accumulation path.
func TestReloadShards_NoPurge_SkipsPurgeLoop(t *testing.T) {
	t.Parallel()
	fake := newFakeReloadClient()

	err := reloadShards(context.Background(), fake, reloadTestConfig(), reloadTestDeployOpts(), false)

	require.NoError(t, err)
	assert.Empty(t, fake.undeployed)
	assert.Empty(t, fake.purgeArgs)
	assert.Empty(t, fake.redeployed)
	assert.Equal(t, []string{"prune", "deploy", "waitReady"}, fake.callLog)
	require.Len(t, fake.deployOptsSeen, 1)
	assert.Equal(t, 1, fake.waitReadyCalls)
}

// TestReloadShards_PurgeShardStateFailure_StillReappliesCRAndRollsImages is the
// regression for the purge-path early-return bug. A single transient
// PurgeShardState failure must not skip DeployShard for the in-flight shard
// (or its ShardPool CR stays deleted -> the shard is down) and must not skip
// the trailing cli.Deploy (or no freshly built image rolls to any shard). The
// wipe error is surfaced, but only after the world is made runnable.
func TestReloadShards_PurgeShardStateFailure_StillReappliesCRAndRollsImages(t *testing.T) {
	t.Parallel()
	fake := newFakeReloadClient()
	fake.purgeErrs["game-2"] = errPurgeBoom

	err := reloadShards(context.Background(), fake, reloadTestConfig(), reloadTestDeployOpts(), true)

	require.Error(t, err)
	require.ErrorIs(t, err, errPurgeBoom)
	assert.Contains(t, err.Error(), "purge shards")
	assert.Contains(t, err.Error(), `failed to wipe state for "game-2"`)

	// The in-flight shard's CR is re-applied despite the wipe failure, so it is
	// not left undeployed (the bug left it with no ShardPool CR).
	assert.Contains(t, fake.redeployed, "game")
	assert.Contains(t, fake.redeployed, "lobby")

	// The trailing image roll still runs for every targeted shard, so the
	// freshly built code is not abandoned for the whole world.
	require.Len(t, fake.deployOptsSeen, 1)
	assert.Equal(t, []string{"game", "lobby"}, shardIDs(fake.deployOptsSeen[0].Shards))
	// The image roll succeeded, so readiness is still awaited.
	assert.Equal(t, 1, fake.waitReadyCalls)

	// The other instance of the same shard is still wiped (the instance loop is
	// not bailed on the first failure) and lobby is purged as usual.
	assert.Equal(t, []string{"game", "game-2", "lobby"}, instances(fake.purgeArgs))
}

// TestReloadShards_PurgeShardStateFailure_CallOrderContinuesPastFailure pins
// the exact call sequence after a mid-shard wipe failure. The early-return bug
// would stop the log at "purge:game-2"; the fix keeps going: re-apply game's CR,
// fully purge+re-apply lobby, then prune -> deploy -> waitReady unchanged.
func TestReloadShards_PurgeShardStateFailure_CallOrderContinuesPastFailure(t *testing.T) {
	t.Parallel()
	fake := newFakeReloadClient()
	fake.purgeErrs["game-2"] = errPurgeBoom

	_ = reloadShards(context.Background(), fake, reloadTestConfig(), reloadTestDeployOpts(), true)

	assert.Equal(t, happyPathCallLog, fake.callLog)
}

// When both a purge and the trailing Deploy fail, both errors surface together
// so the deploy error doesn't mask the incomplete wipe (and vice versa) — the
// "fail after the world is runnable" ordering surfaces neither at the other's
// expense.
func TestReloadShards_PurgeAndDeployBothFail_SurfaceBoth(t *testing.T) {
	t.Parallel()
	fake := newFakeReloadClient()
	fake.purgeErrs["game-2"] = errPurgeBoom
	fake.deployErr = errDeployBoom

	err := reloadShards(context.Background(), fake, reloadTestConfig(), reloadTestDeployOpts(), true)

	require.Error(t, err)
	require.ErrorIs(t, err, errPurgeBoom)
	require.ErrorIs(t, err, errDeployBoom)
	assert.Contains(t, err.Error(), "purge shards")
	assert.Contains(t, err.Error(), "failed to deploy shards")
	// The image roll was attempted (and failed), so readiness is NOT awaited.
	assert.Equal(t, 0, fake.waitReadyCalls)
	// And the CR re-apply still ran for the in-flight shard before the trailing
	// step, so the shard is not left undeployed.
	assert.Equal(t, []string{"game", "lobby"}, fake.redeployed)
}

// A trailing Deploy failure is surfaced and readiness is NOT awaited — Deploy
// is what stamps the image the wait would gate on, so waiting after it failed
// would be a no-op that misleads callers into expecting a runnable world.
func TestReloadShards_DeployFailure_NoWaitReady(t *testing.T) {
	t.Parallel()
	fake := newFakeReloadClient()
	fake.deployErr = errDeployBoom

	err := reloadShards(context.Background(), fake, reloadTestConfig(), reloadTestDeployOpts(), false)

	require.Error(t, err)
	require.ErrorIs(t, err, errDeployBoom)
	assert.Contains(t, err.Error(), "failed to deploy shards")
	assert.Equal(t, 0, fake.waitReadyCalls)
}
