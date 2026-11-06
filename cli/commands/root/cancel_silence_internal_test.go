package root

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// deadSocketSetup builds a Runtime aimed at a nonexistent Docker socket, so any
// daemon call fails with an error instead of needing a running Docker. It returns
// the runtime, docker client, and config that reloadShards drives.
func deadSocketSetup(t *testing.T) (*local.Runtime, *docker.Client, *service.Config) {
	t.Helper()
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/docker.sock")
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}}
	dc, err := docker.NewClient(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dc.Close() })
	return local.New(dc, cfg), dc, cfg
}

// TestReloadShards_CancelSilencedByGate drives reloadShards (shared by `world
// reload` and the log picker's reload hotkey, which prints its errors directly
// in logs.go) with a canceled ctx, then asserts on the returned error via
// ShouldPrint. These tests do not invoke main or observe output/Sentry.
func TestReloadShards_CancelSilencedByGate(t *testing.T) {
	rt, dc, cfg := deadSocketSetup(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := reloadShards(ctx, rt, dc, cfg, nil, false)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"reloadShards must surface the wrapped context.Canceled from CheckLayout, got %v", err)
	assert.False(t, errorspkg.ShouldPrint(err), "a canceled reload must not be printed or Sentry-captured")
}

// TestReloadShards_NonCancelErrorPrinted verifies a real (non-cancel) Docker
// error from CheckLayout against a dead socket passes through reloadShards
// unchanged, so ShouldPrint returns true.
func TestReloadShards_NonCancelErrorPrinted(t *testing.T) {
	rt, dc, cfg := deadSocketSetup(t)

	err := reloadShards(context.Background(), rt, dc, cfg, nil, false)
	require.Error(t, err)
	assert.False(t, errors.Is(err, context.Canceled), "dead socket must not produce context.Canceled")
	assert.True(t, errorspkg.ShouldPrint(err), "a real Docker error must still be printed and Sentry-captured")
}

// TestIsRunning_CancelSilencedByGate calls rt.IsRunning and ShouldPrint
// directly. IsRunning returns a wrapped [context.Canceled]
// (ListProjectContainers wraps the returned error), and the central ShouldPrint
// gate silences it even without a call-site wrap.
func TestIsRunning_CancelSilencedByGate(t *testing.T) {
	rt, _, _ := deadSocketSetup(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.IsRunning(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"IsRunning must return a wrapped context.Canceled, got %v", err)
	assert.False(t, errorspkg.ShouldPrint(err),
		"a wrapped cancel from IsRunning must not be printed or Sentry-captured")
}
