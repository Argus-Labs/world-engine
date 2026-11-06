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

// TestReloadCmdCancel_IsRunning_Silenced verifies that a Ctrl+C during the
// pre-dashboard rt.IsRunning call in world reload is silenced: the cancel
// is converted to a SilentError so ShouldPrint returns false and main.go
// neither prints it nor captures it to Sentry.
func TestReloadCmdCancel_IsRunning_Silenced(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/docker.sock")
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}}
	dc, err := docker.NewClient(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dc.Close() })

	rt := local.New(dc, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = rt.IsRunning(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "IsRunning must return context.Canceled, got %v", err)

	// This is the silencing the fix in reload.go applies:
	silenced := errorspkg.NewSilent(err)
	assert.True(t, errorspkg.IsSilent(silenced), "NewSilent must mark the cancel as silent")
	assert.False(t, errorspkg.ShouldPrint(silenced), "silenced cancel must not be printed or Sentry-captured")
	// And the central gate catches it even without NewSilent:
	assert.False(t, errorspkg.ShouldPrint(err), "ShouldPrint must return false for a raw context.Canceled")
}

// TestReloadCmdCancel_CheckLayout_Silenced verifies the same for the
// pre-dashboard rt.CheckLayout call in reloadShards.
func TestReloadCmdCancel_CheckLayout_Silenced(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/docker.sock")
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}}
	dc, err := docker.NewClient(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dc.Close() })

	rt := local.New(dc, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = rt.CheckLayout(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "CheckLayout must return context.Canceled, got %v", err)

	silenced := errorspkg.NewSilent(err)
	assert.True(t, errorspkg.IsSilent(silenced), "NewSilent must mark the cancel as silent")
	assert.False(t, errorspkg.ShouldPrint(silenced), "silenced cancel must not be printed or Sentry-captured")
	assert.False(t, errorspkg.ShouldPrint(err), "ShouldPrint must return false for a raw context.Canceled")
}

// TestReloadCmd_NonCancelError_StillPrinted verifies that a real (non-cancel)
// Docker error from the pre-dashboard calls is still surfaced — the cancel
// check only catches [context.Canceled], not real failures.
func TestReloadCmd_NonCancelError_StillPrinted(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/docker.sock")
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}}
	dc, err := docker.NewClient(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dc.Close() })

	rt := local.New(dc, cfg)

	// A non-canceled context against a dead socket yields a real Docker error,
	// not a context.Canceled. ShouldPrint must return true so it's printed.
	_, err = rt.IsRunning(context.Background())
	require.Error(t, err)
	assert.False(t, errors.Is(err, context.Canceled), "dead socket must not produce context.Canceled")
	assert.True(t, errorspkg.ShouldPrint(err), "a real Docker error must still be printed and Sentry-captured")
}
