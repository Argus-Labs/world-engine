package root

import (
	"context"
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

// TestCancelSilencedByGate drives the two pre-dashboard call sites (reloadShards,
// shared by `world reload` and the log picker's reload hotkey — which prints its
// errors directly in logs.go — and rt.IsRunning) with a canceled ctx, then
// asserts on the returned error via the central ShouldPrint gate. These tests do
// not invoke main or observe output/Sentry.
func TestCancelSilencedByGate(t *testing.T) {
	rt, dc, cfg := deadSocketSetup(t)

	cases := []struct {
		name string
		err  func(ctx context.Context) error
	}{
		{
			"reloadShards",
			func(ctx context.Context) error {
				return reloadShards(ctx, rt, dc, cfg, nil, false)
			},
		},
		{
			"IsRunning",
			func(ctx context.Context) error {
				_, err := rt.IsRunning(ctx)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := tc.err(ctx)
			require.Error(t, err)
			require.ErrorIs(t, err, context.Canceled,
				"the call must surface the wrapped context.Canceled")
			assert.False(t, errorspkg.ShouldPrint(err),
				"a canceled call must not be printed or Sentry-captured")
		})
	}
}

// TestReloadShards_NonCancelErrorPrinted verifies a real (non-cancel) Docker
// error from CheckLayout against a dead socket passes through reloadShards
// unchanged, so ShouldPrint returns true.
func TestReloadShards_NonCancelErrorPrinted(t *testing.T) {
	rt, dc, cfg := deadSocketSetup(t)

	err := reloadShards(context.Background(), rt, dc, cfg, nil, false)
	require.Error(t, err)
	assert.NotErrorIs(t, err, context.Canceled, "dead socket must not produce context.Canceled")
	assert.True(t, errorspkg.ShouldPrint(err), "a real Docker error must still be printed and Sentry-captured")
}
