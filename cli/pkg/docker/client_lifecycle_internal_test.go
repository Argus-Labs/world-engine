package docker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// fakeContainerClient is a stand-in dockerContainerClient for unit-testing the
// readiness loop without a running daemon. ContainerInspect returns each state
// in states in order, repeating the last on subsequent calls; inspectFn, when
// set, overrides states entirely. It is safe for concurrent use, though
// waitContainerReady polls from a single goroutine.
type fakeContainerClient struct {
	mu        sync.Mutex
	states    []*container.State
	inspectFn func() (client.ContainerInspectResult, error)
	calls     int
}

func (f *fakeContainerClient) ContainerInspect(
	_ context.Context,
	_ string,
	_ client.ContainerInspectOptions,
) (client.ContainerInspectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.inspectFn != nil {
		return f.inspectFn()
	}
	var st *container.State
	if len(f.states) > 0 {
		idx := f.calls - 1
		if idx >= len(f.states) {
			idx = len(f.states) - 1
		}
		st = f.states[idx]
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{State: st}}, nil
}

// state builders modeling the Docker states waitContainerReady branches on.
// A restart-policy crash loop reports "restarting" (never "exited"); on some
// daemons Running stays true throughout that window, so both variants exist.
func restartingState(exitCode int) *container.State {
	return &container.State{Status: container.StateRestarting, Running: true, Restarting: true, ExitCode: exitCode}
}

func restartingNotRunningState(exitCode int) *container.State {
	return &container.State{Status: container.StateRestarting, Running: false, Restarting: true, ExitCode: exitCode}
}

func restartingWithHealth(exitCode int, health container.HealthStatus) *container.State {
	return &container.State{
		Status:     container.StateRestarting,
		Running:    true,
		Restarting: true,
		ExitCode:   exitCode,
		Health:     &container.Health{Status: health},
	}
}

func exitedState(exitCode int) *container.State {
	return &container.State{Status: container.StateExited, Running: false, ExitCode: exitCode}
}

func runningNoProbeState() *container.State {
	return &container.State{Status: container.StateRunning, Running: true}
}

func healthyState() *container.State {
	return &container.State{
		Status:  container.StateRunning,
		Running: true,
		Health:  &container.Health{Status: container.Healthy},
	}
}

func startingHealthState() *container.State {
	return &container.State{
		Status:  container.StateRunning,
		Running: true,
		Health:  &container.Health{Status: container.Starting},
	}
}

func createdState() *container.State {
	return &container.State{Status: container.StateCreated, Running: false}
}

// TestWaitForReady pins the crash-loop fast-fail fix and its non-regressions.
// Before the fix, a crash-looping unless-stopped container (every platform
// container) reported "restarting" — never "exited" — and stayed Running=true on
// some daemons, so the exited fast-fail was dead code: probe-based services
// (NATS/ReadyURL, Postgres/healthcheck) hung for the full readyTimeout, and a
// probe-less service hit the switch's default and was declared ready instantly.
func TestWaitForReady(t *testing.T) {
	t.Parallel()

	const (
		poll      = 10 * time.Millisecond // fast, deterministic ticks
		svcName   = "demo-svc"
		waitCtx   = 500 * time.Millisecond // bounds the "keep waiting" cases
		fastMax   = 1 * time.Second        // fast-fail/ready must return well under this
		waitMax   = 3 * time.Second
		callerCtx = 10 * time.Second // upper bound for the fast-fail/ready cases
	)

	cases := map[string]struct {
		states         []*container.State
		inspectFn      func() (client.ContainerInspectResult, error)
		readyURLStatus int // 0 = no ReadyURL; >0 = spin an httptest server returning it
		ctxWithin      time.Duration
		wantReady      bool
		wantErrSub     string
		wantNoErrSub   string
		wantLogCount   int
		wantCalls      int // 0 = don't assert; >0 = assert exactly that many inspect calls
		maxElapsed     time.Duration
	}{
		// --- THE FIX: crash-looping restart-policy containers fail fast ---
		"CrashLoopFailsFast_ProbeLessService": {
			// A probe-less service crash-looping must NOT be declared ready.
			states:       []*container.State{restartingState(2)},
			ctxWithin:    callerCtx,
			wantErrSub:   "exited with code 2",
			wantNoErrSub: "timeout waiting",
			wantLogCount: 1,
			wantCalls:    1,
			maxElapsed:   fastMax,
		},
		"CrashLoopFailsFast_ReadyURLService": {
			// NATS-style: while dead the ReadyURL never answers, so it used to hang.
			states:         []*container.State{restartingState(127)},
			readyURLStatus: http.StatusServiceUnavailable,
			ctxWithin:      callerCtx,
			wantErrSub:     "exited with code 127",
			wantNoErrSub:   "timeout waiting",
			wantLogCount:   1,
			wantCalls:      1,
			maxElapsed:     fastMax,
		},
		"CrashLoopFailsFast_HealthcheckService": {
			// Postgres-style: a healthcheck can never succeed while dead.
			states:       []*container.State{restartingWithHealth(137, container.Starting)},
			ctxWithin:    callerCtx,
			wantErrSub:   "exited with code 137",
			wantNoErrSub: "timeout waiting",
			wantLogCount: 1,
			wantCalls:    1,
			maxElapsed:   fastMax,
		},
		"CrashLoopRunningFalseAlsoFailsFast": {
			// Daemons that set Running=false during the restart window.
			states:       []*container.State{restartingNotRunningState(5)},
			ctxWithin:    callerCtx,
			wantErrSub:   "exited with code 5",
			wantLogCount: 1,
			wantCalls:    1,
			maxElapsed:   fastMax,
		},

		// --- no regression: the original exited fast-fail (no restart policy) ---
		"ExitedNonzeroFailsFast": {
			states:       []*container.State{exitedState(1)},
			ctxWithin:    callerCtx,
			wantErrSub:   "exited with code 1",
			wantLogCount: 1,
			wantCalls:    1,
			maxElapsed:   fastMax,
		},
		"ExitedZeroFailsFastToo": {
			// The exited branch is unchanged: any exit code fails fast (the
			// ExitCode != 0 gate applies only to the restarting branch).
			states:       []*container.State{exitedState(0)},
			ctxWithin:    callerCtx,
			wantErrSub:   "exited with code 0",
			wantLogCount: 1,
			wantCalls:    1,
			maxElapsed:   fastMax,
		},

		// --- gate: a clean (exit 0) restart loop is not mistaken for a crash ---
		"RestartingZeroExitDoesNotFailFast": {
			states:       []*container.State{restartingState(0)},
			ctxWithin:    waitCtx,
			wantErrSub:   "timeout waiting",
			wantNoErrSub: "exited with code",
			wantLogCount: 1,
			maxElapsed:   waitMax,
		},

		// --- no regression: happy paths stay ready ---
		"HealthyHealthcheckIsReady": {
			states:       []*container.State{healthyState()},
			ctxWithin:    callerCtx,
			wantReady:    true,
			wantLogCount: 0,
			wantCalls:    1,
			maxElapsed:   fastMax,
		},
		"ProbeLessRunningIsReady": {
			states:       []*container.State{runningNoProbeState()},
			ctxWithin:    callerCtx,
			wantReady:    true,
			wantLogCount: 0,
			wantCalls:    1,
			maxElapsed:   fastMax,
		},
		"ReadyURLReadyIsReady": {
			states:         []*container.State{runningNoProbeState()},
			readyURLStatus: http.StatusOK,
			ctxWithin:      callerCtx,
			wantReady:      true,
			wantLogCount:   0,
			wantCalls:      1,
			maxElapsed:     fastMax,
		},

		// --- no false-ready while still starting / probe unhealthy ---
		"StartingHealthcheckWaitsThenTimesOut": {
			states:       []*container.State{startingHealthState()},
			ctxWithin:    waitCtx,
			wantErrSub:   "timeout waiting",
			wantNoErrSub: "exited with code",
			wantLogCount: 1,
			maxElapsed:   waitMax,
		},
		"ReadyURLNotReadyWaitsThenTimesOut": {
			states:         []*container.State{runningNoProbeState()},
			readyURLStatus: http.StatusServiceUnavailable,
			ctxWithin:      waitCtx,
			wantErrSub:     "timeout waiting",
			wantNoErrSub:   "exited with code",
			wantLogCount:   1,
			maxElapsed:     waitMax,
		},
		"NilStateWaitsThenTimesOut": {
			states:       []*container.State{nil},
			ctxWithin:    waitCtx,
			wantErrSub:   "timeout waiting",
			wantNoErrSub: "exited with code",
			wantLogCount: 1,
			maxElapsed:   waitMax,
		},
		"CreatedStateWaitsThenTimesOut": {
			// A freshly-created container must not be mistaken for an exited crash.
			states:       []*container.State{createdState()},
			ctxWithin:    waitCtx,
			wantErrSub:   "timeout waiting",
			wantNoErrSub: "exited with code",
			wantLogCount: 1,
			maxElapsed:   waitMax,
		},

		// --- inspect errors propagate immediately (no logging) ---
		"InspectErrorPropagates": {
			inspectFn: func() (client.ContainerInspectResult, error) {
				return client.ContainerInspectResult{}, errors.New("daemon unreachable")
			},
			ctxWithin:    callerCtx,
			wantErrSub:   "daemon unreachable",
			wantNoErrSub: "timeout waiting",
			wantLogCount: 0,
			wantCalls:    1,
			maxElapsed:   fastMax,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeContainerClient{states: tc.states, inspectFn: tc.inspectFn}
			svc := service.Service{Name: svcName}
			if tc.readyURLStatus > 0 {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.readyURLStatus)
				}))
				t.Cleanup(srv.Close)
				svc.ReadyURL = srv.URL + "/healthz"
			}

			var mu sync.Mutex
			logged := make([]string, 0, 1)
			logDebug := func(name string) {
				mu.Lock()
				logged = append(logged, name)
				mu.Unlock()
			}

			ctx, cancel := context.WithTimeout(context.Background(), tc.ctxWithin)
			defer cancel()

			start := time.Now()
			err := waitContainerReady(ctx, fake, logDebug, poll, svc)
			elapsed := time.Since(start)

			if tc.wantReady {
				require.NoError(t, err, "expected %s to become ready", svcName)
			} else {
				require.Error(t, err, "expected an error; nil would silently declare a dead container ready")
				require.Contains(t, err.Error(), tc.wantErrSub, "error content")
				if tc.wantNoErrSub != "" {
					assert.NotContains(t, err.Error(), tc.wantNoErrSub, "must not contain")
				}
			}
			require.Len(t, logged, tc.wantLogCount, "logDebug call count")
			if len(logged) > 0 {
				assert.Equal(t, svcName, logged[0], "logDebug received the container name")
			}
			if tc.wantCalls > 0 {
				assert.Equal(t, tc.wantCalls, fake.calls, "inspect call count (one poll then a decision)")
			}
			assert.True(t, elapsed < tc.maxElapsed,
				"returned in %s, expected < %s (fast-fail vs %s hang)", elapsed, tc.maxElapsed, readyTimeout)
		})
	}
}

// TestHTTPReady pins the readiness probe waitContainerReady relies on for
// ReadyURL-based services (NATS): only a 2xx reply proves the process inside is
// serving, since Docker's port proxy accepts a TCP connection on container start.
func TestHTTPReady(t *testing.T) {
	t.Parallel()

	t.Run("2xxIsReady", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		assert.True(t, httpReady(ctx, srv.URL+"/healthz"))
	})

	t.Run("Non2xxIsNotReady", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(srv.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		assert.False(t, httpReady(ctx, srv.URL+"/healthz"))
	})

	t.Run("ServerDownIsNotReady", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		addr := srv.URL
		srv.Close() // immediately torn down -> connection refused
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		assert.False(t, httpReady(ctx, addr+"/healthz"))
	})
}
