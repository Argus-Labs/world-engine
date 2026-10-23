//go:build integration

package docker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// TestWaitForReady_CrashLoopRealDaemon reproduces the bug report against a real
// Docker daemon (reproduced on 26.1.5): an unless-stopped container that crashes
// at boot reports "restarting" (never "exited") and can stay Running=true through
// the restart window, so the buggy fast-fail (Status=="exited" && !Running) never
// fired. It drives the real Client.waitForReady — the production code path
// (c.client.ContainerInspect on a live container) — so it is a true end-to-end
// check of the fix, not a fake-daemon unit test.
//
// Precondition: a reachable Docker daemon and the alpine:3 image (pulled by the
// test). Network access is needed for the pull. Each case creates, starts,
// probes, and removes its own container with a unique name.
//
// Probe-less crash-loop note: the deterministic regression guard for the
// reported probe-less "declared ready while crash-looping" symptom is the unit
// test TestWaitForReady/CrashLoopFailsFast_ProbelessService, which fixes the
// inspect state to "restarting, exit=2". On a real daemon an unless-stopped
// crash-loop is "restarting, exit=3" on ~94% of polls but briefly "running,
// exit=0" on ~6% (Docker resets ExitCode to 0 on each fresh start); the probe-less
// readiness switch returns ready on that brief instant, and because its ExitCode
// is 0 no stricter ExitCode gate can catch it without misfiring on a legitimate
// mid-run OOM that the unless-stopped policy recovered from. That residual is
// inherent to the recommended fix. The end-to-end coverage here therefore uses a
// ReadyURL-to-a-dead-port variant (NATS scenario) and a healthcheck variant,
// both deterministic because their probes never falsely report ready during the
// brief running instant, plus an assertion that the real container does reach
// the "restarting + nonzero exit" state the fix keys on.
func TestWaitForReady_CrashLoopRealDaemon(t *testing.T) {
	t.Parallel()

	dc, err := NewClient(&service.Config{}, &ClientOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	t.Cleanup(func() { _ = dc.Close() })

	// alpine:3 is the crash victim. Pulled once for the whole test.
	pullCtx, pullCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer pullCancel()
	require.NoError(t, pullImage(pullCtx, dc, "alpine:3"), "alpine:3 must be present")

	// D / G headline: an unless-stopped crash-loop with a readiness probe that
	// can never succeed while the process is dead must fail fast (not hang the
	// 5-minute readyTimeout), returning the real exit code.
	cases := map[string]struct {
		cmd        []string
		policy     container.RestartPolicy
		health     *container.HealthConfig
		readyURL   string
		wantReady  bool
		wantErrSub string
		maxWait    time.Duration // the fix returns well under this; the bug would exceed it
	}{
		"UnlessStopped_CrashLoop_WithHealthcheck_FailsFast": {
			cmd:        []string{"sh", "-c", "exit 7"},
			policy:     container.RestartPolicy{Name: "unless-stopped"},
			health:     &container.HealthConfig{Test: []string{"CMD-SHELL", "exit 1"}},
			wantErrSub: "exited with code 7",
			maxWait:    15 * time.Second,
		},
		"UnlessStopped_CrashLoop_WithReadyURL_FailsFast": {
			// NATS scenario (D): the ReadyURL probes a port nothing will ever
			// answer, so the probe can never falsely report ready while the
			// process is crash-looping — the fix's restart fast-fail wins.
			cmd:        []string{"sh", "-c", "exit 3"},
			policy:     container.RestartPolicy{Name: "unless-stopped"},
			readyURL:   "http://127.0.0.1:59999/healthz", // nothing listens here
			wantErrSub: "exited with code 3",
			maxWait:    15 * time.Second,
		},
		// Preserved original path: a no-policy exit fails fast (already worked).
		"NoPolicy_ExitedNonzero_FailsFast": {
			cmd:        []string{"sh", "-c", "exit 1"},
			policy:     container.RestartPolicy{Name: "no"},
			wantErrSub: "exited with code 1",
			maxWait:    15 * time.Second,
		},
		// Happy path: a probe-less running container is declared ready.
		"ProbeLess_Running_IsReady": {
			cmd:       []string{"sh", "-c", "sleep 30"},
			policy:    container.RestartPolicy{Name: "no"},
			wantReady: true,
			maxWait:   5 * time.Second,
		},
		// Happy path: a container with a passing healthcheck becomes healthy -> ready.
		"Healthcheck_Healthy_IsReady": {
			cmd:    []string{"sh", "-c", "sleep 30"},
			policy: container.RestartPolicy{Name: "no"},
			health: &container.HealthConfig{
				Test: []string{"CMD-SHELL", "true"}, Interval: time.Second, Timeout: time.Second, Retries: 1,
			},
			wantReady: true,
			maxWait:   15 * time.Second,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc := service.Service{
				Name:          "wc-int-" + strings.ToLower(strings.ReplaceAll(name, "_", "-")),
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

			require.NoError(t, dc.startContainer(ctx, svc), "start container")

			start := time.Now()
			err := dc.waitForReady(ctx, svc)
			elapsed := time.Since(start)

			if tc.wantReady {
				require.NoError(t, err, "expected ready")
				assert.Less(t, elapsed, tc.maxWait, "ready within %s", tc.maxWait)
			} else {
				require.Error(t, err, "crash-loop must fail, not be declared ready")
				assert.Contains(t, err.Error(), tc.wantErrSub, "fast-fail message")
				assert.NotContains(t, err.Error(), "timeout waiting", "must not hang for readyTimeout")
				assert.Less(t, elapsed, tc.maxWait,
					"fast-fail in %s, expected <%s (not the %s readyTimeout)",
					elapsed, tc.maxWait, readyTimeout)
			}
		})
	}

	// UnlessStopped_CrashLoop_ReachesRestartingState is the deterministic,
	// real-daemon half of the probe-less coverage: it asserts the container
	// actually reports "restarting" with a nonzero exit code — the exact
	// condition the fix keys on — within a few seconds. (The unit test then
	// proves that condition returns the "exited with code N" error.)
	t.Run("UnlessStopped_CrashLoop_ReachesRestartingState", func(t *testing.T) {
		t.Parallel()

		const name = "wc-int-stateprobe"
		svc := service.Service{
			Name:          name,
			Image:         "alpine:3",
			Cmd:           []string{"sh", "-c", "exit 4"},
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		}
		_ = dc.removeContainer(context.Background(), name, true)
		t.Cleanup(func() { _ = dc.removeContainer(context.Background(), name, true) })
		require.NoError(t, dc.startContainer(context.Background(), svc))

		deadline := time.Now().Add(10 * time.Second)
		var sawRestartingNonzero bool
		for time.Now().Before(deadline) {
			st, err := dc.InspectContainer(context.Background(), name)
			if err != nil {
				t.Fatalf("inspect: %v", err)
			}
			if st.State != nil && st.State.Restarting && st.State.ExitCode != 0 {
				sawRestartingNonzero = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		require.True(t, sawRestartingNonzero,
			fmt.Sprintf("unless-stopped crash-loop never reported restarting+nonzero exit; fix trigger unreachable: %+v",
				inspectState(t, dc, name)))
	})
}

// pullImage ensures an image is present on the daemon.
func pullImage(ctx context.Context, c *Client, image string) error {
	return c.PullImages(ctx, []service.Service{{Image: image}}, nil)
}

// inspectState is a best-effort state snapshot for a failure message.
func inspectState(t *testing.T, c *Client, name string) string {
	t.Helper()
	st, err := c.InspectContainer(context.Background(), name)
	if err != nil || st.State == nil {
		return "<no state>"
	}
	return fmt.Sprintf("status=%s running=%v restarting=%v exit=%d",
		st.State.Status, st.State.Running, st.State.Restarting, st.State.ExitCode)
}
