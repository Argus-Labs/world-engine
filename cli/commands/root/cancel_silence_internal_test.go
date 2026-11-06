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
// the runtime and the docker client that reloadShards drives.
func deadSocketSetup(t *testing.T) (*local.Runtime, *docker.Client, *service.Config) {
	t.Helper()
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/docker.sock")
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}}
	dc, err := docker.NewClient(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dc.Close() })
	return local.New(dc, cfg), dc, cfg
}

// TestReloadCmdCancel_ReloadShardsSilenced drives the reload command's actual
// guard-bearing function (reloadShards, shared by `world reload` and the log
// picker's reload hotkey) instead of replicating its logic. A Ctrl+C during the
// pre-dashboard rt.CheckLayout call is surfaced as a wrapped context.Canceled
// (ListProjectContainers eris-wraps the returned error), and reload.go's guard
// wraps it in NewSilent so ShouldPrint returns false and main.go neither prints
// it nor captures it to Sentry.
func TestReloadCmdCancel_ReloadShardsSilenced(t *testing.T) {
	rt, dc, cfg := deadSocketSetup(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := reloadShards(ctx, rt, dc, cfg, nil, false)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"reloadShards must surface the wrapped context.Canceled from CheckLayout, got %v", err)
	assert.True(t, errorspkg.IsSilent(err), "the reload.go CheckLayout guard must wrap the cancel in NewSilent")
	assert.False(t, errorspkg.ShouldPrint(err), "a canceled reload must not be printed or Sentry-captured")
}

// TestReloadCmd_NonCancelError_StillPrinted verifies the guard only catches
// context.Canceled: a real (non-cancel) Docker error from CheckLayout against a
// dead socket passes through reloadShards unchanged, so ShouldPrint returns true
// and it is still printed and Sentry-captured.
func TestReloadCmd_NonCancelError_StillPrinted(t *testing.T) {
	rt, dc, cfg := deadSocketSetup(t)

	err := reloadShards(context.Background(), rt, dc, cfg, nil, false)
	require.Error(t, err)
	assert.False(t, errors.Is(err, context.Canceled), "dead socket must not produce context.Canceled")
	assert.False(t, errorspkg.IsSilent(err), "a real Docker error must not be marked silent")
	assert.True(t, errorspkg.ShouldPrint(err), "a real Docker error must still be printed and Sentry-captured")
}

// TestReloadCmdCancel_IsRunningSilencedByGate verifies the silencing of a Ctrl+C
// during the pre-dashboard rt.IsRunning call in `world reload` and `world logs`.
// IsRunning returns a wrapped context.Canceled (ListProjectContainers wraps the
// returned error); the command-level guards wrap it in NewSilent, and the
// central ShouldPrint gate silences it even without that wrap — this is the
// actual main.go path. Asserted against IsRunning directly so the test reports
// the runtime's real cancel form rather than reconstructing NewSilent itself.
func TestReloadCmdCancel_IsRunningSilencedByGate(t *testing.T) {
	rt, _, _ := deadSocketSetup(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.IsRunning(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"IsRunning must return a wrapped context.Canceled, got %v", err)
	assert.False(t, errorspkg.ShouldPrint(err),
		"a raw cancel from IsRunning must not be printed or Sentry-captured")
}
