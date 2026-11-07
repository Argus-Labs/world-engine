package root

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// fakeShardBuilder is a stand-in shardBuilder for unit-testing the build path
// without a running daemon. It records every method call in arrival order so
// tests can assert pull-before-build ordering, and returns caller-configured
// results/errors.
type fakeShardBuilder struct {
	mu           sync.Mutex
	imagesToPull []string
	refsToPull   []string
	pullErr      error
	pullRefsErr  error
	buildErr     error
	calls        []string
}

func (f *fakeShardBuilder) ImagesToPull(_ context.Context, _ []service.Service) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "ImagesToPull")
	return f.imagesToPull
}

func (f *fakeShardBuilder) RefsToPull(_ context.Context, _ []string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "RefsToPull")
	return f.refsToPull
}

func (f *fakeShardBuilder) PullImages(_ context.Context, _ []service.Service, _ func(docker.Progress)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "PullImages")
	return f.pullErr
}

func (f *fakeShardBuilder) PullImageRefs(_ context.Context, _ []string, _ func(docker.Progress)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "PullImageRefs")
	return f.pullRefsErr
}

func (f *fakeShardBuilder) BuildCardinalImages(
	_ context.Context,
	_ []service.Service,
	_ func(docker.Progress),
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "BuildCardinalImages")
	return f.buildErr
}

func (f *fakeShardBuilder) callOrder() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// testShardServices builds one Cardinal shard service for the "demo" project,
// matching what BuildCmd.Run produces for a single-shard world.toml.
func testShardServices() []service.Service {
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}}
	return []service.Service{
		service.CardinalFromShard(
			cfg,
			worldtoml.Shard{ID: "game", InstanceID: "game", Path: "shards/game"},
			8081,
		),
	}
}

// TestBuildShards_ColdCachePullsBeforeBuild is the primary regression guard:
// on a cold Docker cache (ImagesToPull returns missing FROM images) the build
// path must pull those images before building, or BuildKit fails with the
// opaque "no active sessions". Before the fix, buildShards called
// buildShardImages directly with no preceding pull.
func TestBuildShards_ColdCachePullsBeforeBuild(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dash := phasebox.Start(ctx, phasebox.Plain)
	defer dash.Complete()

	fake := &fakeShardBuilder{
		imagesToPull: []string{
			"golang:1.27.1-bookworm",
			"gcr.io/distroless/base-debian12",
		},
	}

	require.NoError(t, buildShards(ctx, dash, fake, testShardServices()))

	calls := fake.callOrder()
	pullIdx := slices.Index(calls, "PullImages")
	buildIdx := slices.Index(calls, "BuildCardinalImages")
	require.GreaterOrEqual(t, pullIdx, 0, "pull must run on a cold cache")
	require.GreaterOrEqual(t, buildIdx, 0, "build must run")
	assert.Less(t, pullIdx, buildIdx, "pull must precede build (cold cache)")
}

// TestBuildShards_WarmCacheSkipsPullStillBuilds verifies that when all FROM
// images are cached (ImagesToPull returns empty) the "Image Pull" phase is
// skipped entirely — PullImages is never called — but the build still runs.
// This is the repeat-build case on CI runners and local developer machines;
// skipping the pull keeps clean CI output (no "0 images pulled" box).
func TestBuildShards_WarmCacheSkipsPullStillBuilds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dash := phasebox.Start(ctx, phasebox.Plain)
	defer dash.Complete()

	fake := &fakeShardBuilder{
		imagesToPull: nil,
	}

	require.NoError(t, buildShards(ctx, dash, fake, testShardServices()))

	calls := fake.callOrder()
	assert.NotContains(t, calls, "PullImages", "pull must be skipped on a warm cache")
	assert.Contains(t, calls, "BuildCardinalImages", "build must still run on a warm cache")
}

// TestBuildShards_PullErrorPreventsBuild verifies that a pull failure stops the
// build from running. The whole point of pre-pulling is to surface dependency
// errors before BuildKit gets a chance to fail with the opaque "no active
// sessions"; if the build ran anyway, a pull error would be silently followed
// by a build crash.
func TestBuildShards_PullErrorPreventsBuild(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dash := phasebox.Start(ctx, phasebox.Plain)
	defer dash.Complete()

	pullErr := errors.New("network unreachable")
	fake := &fakeShardBuilder{
		imagesToPull: []string{"golang:1.27.1-bookworm"},
		pullErr:      pullErr,
	}

	err := buildShards(ctx, dash, fake, testShardServices())
	require.Error(t, err)
	assert.ErrorIs(t, err, pullErr)
	assert.Contains(t, err.Error(), "pull build dependencies")

	calls := fake.callOrder()
	assert.NotContains(t, calls, "BuildCardinalImages",
		"build must not run after a pull failure")
}

// TestBuildShards_BuildErrorPropagates verifies a build failure is returned
// (wrapped with the build message) and not swallowed. Uses a warm cache so
// the pull phase is a no-op and only the build path is exercised.
func TestBuildShards_BuildErrorPropagates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dash := phasebox.Start(ctx, phasebox.Plain)
	defer dash.Complete()

	buildErr := errors.New("docker build failed")
	fake := &fakeShardBuilder{
		imagesToPull: nil,
		buildErr:     buildErr,
	}

	err := buildShards(ctx, dash, fake, testShardServices())
	require.Error(t, err)
	assert.ErrorIs(t, err, buildErr)
	assert.Contains(t, err.Error(), "Failed to build Cardinal images")
}

// TestBuildShards_NoExtraImageRefs verifies the build path never calls
// PullImageRefs: it has no platform images (extraImageRefs is nil), unlike
// world start which passes platform images. This guards against accidentally
// wiring platform image refs into the CI-only build command.
func TestBuildShards_NoExtraImageRefs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dash := phasebox.Start(ctx, phasebox.Plain)
	defer dash.Complete()

	fake := &fakeShardBuilder{
		imagesToPull: []string{"golang:1.27.1-bookworm"},
	}

	require.NoError(t, buildShards(ctx, dash, fake, testShardServices()))

	calls := fake.callOrder()
	assert.NotContains(t, calls, "PullImageRefs",
		"build path must not pull platform images (no extraImageRefs)")
}
