//go:build integration

package docker

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// TestWaitForReady_CrashLoopRealDaemon drives the real waitForReady against live
// containers: unless-stopped crash loops must fail fast with the exit code, with
// or without a probe, and healthy containers must still be declared ready.
func TestWaitForReady_CrashLoopRealDaemon(t *testing.T) {
	t.Parallel()

	dc, err := NewClient(&service.Config{}, &ClientOptions{Logger: slog.New(slog.DiscardHandler)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = dc.Close() })
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if _, err := dc.client.Ping(pingCtx, client.PingOptions{}); err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	pullCtx, pullCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer pullCancel()
	require.NoError(t, dc.PullImages(pullCtx, []service.Service{{Image: "alpine:3"}}, nil))

	unlessStopped := container.RestartPolicy{Name: "unless-stopped"}
	run := strconv.FormatInt(time.Now().UnixNano()%1_000_000, 36) // concurrent runs must not share names
	cases := map[string]struct {
		cmd        []string
		policy     container.RestartPolicy
		health     *container.HealthConfig
		readyURL   string
		wantReady  bool
		wantErrSub string
		maxWait    time.Duration
	}{
		"UnlessStopped_CrashLoop_WithHealthcheck_FailsFast": {
			cmd:        []string{"sh", "-c", "exit 7"},
			policy:     unlessStopped,
			health:     &container.HealthConfig{Test: []string{"CMD-SHELL", "exit 1"}},
			wantErrSub: "crash-looping",
			maxWait:    30 * time.Second,
		},
		"UnlessStopped_CrashLoop_WithReadyURL_FailsFast": {
			cmd:        []string{"sh", "-c", "exit 3"},
			policy:     unlessStopped,
			readyURL:   "http://127.0.0.1:59999/healthz", // nothing listens here
			wantErrSub: "crash-looping",
			maxWait:    30 * time.Second,
		},
		"UnlessStopped_CrashLoop_ProbeLess_FailsFast": {
			cmd:        []string{"sh", "-c", "exit 4"},
			policy:     unlessStopped,
			wantErrSub: "crash-looping",
			maxWait:    30 * time.Second,
		},
		"UnlessStopped_SlowCrash_ProbeLess_FailsFast": {
			// Dies after the first poll saw it running; must not be declared ready.
			cmd:        []string{"sh", "-c", "sleep 1.5; exit 5"},
			policy:     unlessStopped,
			wantErrSub: "crash-looping", // the exit code is only seen when a poll lands mid-restart
			maxWait:    60 * time.Second,
		},
		"NoPolicy_ExitedNonzero_FailsFast": {
			cmd:        []string{"sh", "-c", "exit 1"},
			policy:     container.RestartPolicy{Name: "no"},
			wantErrSub: "exited with code 1",
			maxWait:    30 * time.Second,
		},
		"ProbeLess_Running_IsReady": {
			cmd:       []string{"sh", "-c", "sleep 60"},
			policy:    unlessStopped,
			wantReady: true,
			maxWait:   15 * time.Second,
		},
		"Healthcheck_Healthy_IsReady": {
			cmd:    []string{"sh", "-c", "sleep 60"},
			policy: unlessStopped,
			health: &container.HealthConfig{
				Test: []string{"CMD-SHELL", "true"}, Interval: time.Second, Timeout: time.Second, Retries: 1,
			},
			wantReady: true,
			maxWait:   30 * time.Second,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc := service.Service{
				Name:          "wc-int-" + run + "-" + strings.ToLower(strings.ReplaceAll(name, "_", "-")),
				Image:         "alpine:3",
				Cmd:           tc.cmd,
				RestartPolicy: tc.policy,
				Healthcheck:   tc.health,
				ReadyURL:      tc.readyURL,
			}
			_ = dc.removeContainer(context.Background(), svc.Name, true)
			t.Cleanup(func() { _ = dc.removeContainer(context.Background(), svc.Name, true) })

			ctx, cancel := context.WithTimeout(context.Background(), tc.maxWait)
			defer cancel()
			startAt := time.Now()
			require.NoError(t, dc.startContainer(ctx, svc), "start container")
			t.Logf("startContainer took %s", time.Since(startAt))

			start := time.Now()
			err := dc.waitForReady(ctx, svc)
			elapsed := time.Since(start)
			t.Logf("waitForReady took %s (err=%v)", elapsed, err)

			if tc.wantReady {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrSub)
				assert.NotContains(t, err.Error(), "timeout waiting")
			}
			assert.Less(t, elapsed, tc.maxWait)
		})
	}
}
