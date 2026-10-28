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

// fakeContainerClient returns states[i] (and restartCounts[i]) on the i-th inspect,
// repeating the last entry; inspectFn overrides both.
type fakeContainerClient struct {
	mu            sync.Mutex
	states        []*container.State
	restartCounts []int
	inspectFn     func() (client.ContainerInspectResult, error)
	calls         int
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
	idx := f.calls - 1
	var st *container.State
	if len(f.states) > 0 {
		st = f.states[min(idx, len(f.states)-1)]
	}
	restarts := 0
	if len(f.restartCounts) > 0 {
		restarts = f.restartCounts[min(idx, len(f.restartCounts)-1)]
	}
	return client.ContainerInspectResult{
		Container: container.InspectResponse{State: st, RestartCount: restarts},
	}, nil
}

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

func TestWaitForReady(t *testing.T) {
	t.Parallel()

	const (
		poll      = 10 * time.Millisecond
		svcName   = "demo-svc"
		waitCtx   = 500 * time.Millisecond // bounds the "keep waiting" cases
		fastMax   = 1 * time.Second
		waitMax   = 3 * time.Second
		callerCtx = 10 * time.Second
	)
	looping := []int{crashLoopRestarts}

	cases := map[string]struct {
		states         []*container.State
		restartCounts  []int
		inspectFn      func() (client.ContainerInspectResult, error)
		readyURLStatus int // 0 = no ReadyURL; >0 = spin an httptest server returning it
		ctxWithin      time.Duration
		wantReady      bool
		wantErrSub     string
		wantNoErrSub   string
		wantLogCount   int
		wantCalls      int // 0 = don't assert
		maxElapsed     time.Duration
	}{
		// crash loops (restart policy) fail fast once crashLoopRestarts is reached
		"CrashLoopFailsFast_ProbeLessService": {
			states:        []*container.State{restartingState(2)},
			restartCounts: looping,
			ctxWithin:     callerCtx,
			wantErrSub:    "crash-looping: 3 restarts, last exit code 2",
			wantNoErrSub:  "timeout waiting",
			wantLogCount:  1,
			wantCalls:     1,
			maxElapsed:    fastMax,
		},
		"CrashLoopFailsFast_ReadyURLService": {
			states:         []*container.State{restartingState(127)},
			restartCounts:  looping,
			readyURLStatus: http.StatusServiceUnavailable,
			ctxWithin:      callerCtx,
			wantErrSub:     "crash-looping: 3 restarts, last exit code 127",
			wantNoErrSub:   "timeout waiting",
			wantLogCount:   1,
			wantCalls:      1,
			maxElapsed:     fastMax,
		},
		"CrashLoopFailsFast_HealthcheckService": {
			states:        []*container.State{restartingWithHealth(137, container.Starting)},
			restartCounts: looping,
			ctxWithin:     callerCtx,
			wantErrSub:    "crash-looping: 3 restarts, last exit code 137",
			wantNoErrSub:  "timeout waiting",
			wantLogCount:  1,
			wantCalls:     1,
			maxElapsed:    fastMax,
		},
		"CrashLoopRunningFalseAlsoFailsFast": {
			states: []*container.State{restartingNotRunningState(5)}, restartCounts: looping,
			ctxWithin: callerCtx, wantErrSub: "crash-looping: 3 restarts, last exit code 5",
			wantLogCount: 1, wantCalls: 1, maxElapsed: fastMax,
		},
		"ExitZeroLoopAlsoFailsFast": {
			states:        []*container.State{restartingState(0)},
			restartCounts: looping,
			ctxWithin:     callerCtx,
			wantErrSub:    "crash-looping: 3 restarts, last exit code 0",
			wantNoErrSub:  "timeout waiting",
			wantLogCount:  1,
			wantCalls:     1,
			maxElapsed:    fastMax,
		},
		"ProbeLessSlowCrashIsCaught": {
			// Runs ~2 polls, dies, restarts: never stable for stablePolls, fails at the threshold.
			states: []*container.State{
				runningNoProbeState(), runningNoProbeState(), restartingState(1),
				runningNoProbeState(), runningNoProbeState(), restartingState(1),
				runningNoProbeState(), runningNoProbeState(), restartingState(1),
			},
			restartCounts: []int{0, 0, 1, 1, 1, 2, 2, 2, 3},
			ctxWithin:     callerCtx,
			wantErrSub:    "crash-looping: 3 restarts, last exit code 1",
			wantNoErrSub:  "timeout waiting",
			wantLogCount:  1,
			wantCalls:     9,
			maxElapsed:    fastMax,
		},

		// restarts below the threshold keep waiting, and may still become ready
		"RestartingBelowThresholdKeepsWaiting": {
			states: []*container.State{restartingState(1)}, restartCounts: []int{crashLoopRestarts - 1},
			ctxWithin: waitCtx, wantErrSub: "timeout waiting", wantNoErrSub: "crash-looping",
			wantLogCount: 1, maxElapsed: waitMax,
		},
		"TransientRestartThenHealthyIsReady": {
			states: []*container.State{restartingState(1), healthyState()}, restartCounts: []int{1, 1},
			ctxWithin: callerCtx, wantReady: true, wantLogCount: 0, wantCalls: 2, maxElapsed: fastMax,
		},

		// no restart policy: exited fails fast with any code
		"ExitedNonzeroFailsFast": {
			states:    []*container.State{exitedState(1)},
			ctxWithin: callerCtx, wantErrSub: "exited with code 1", wantLogCount: 1, wantCalls: 1, maxElapsed: fastMax,
		},
		"ExitedZeroFailsFastToo": {
			states:    []*container.State{exitedState(0)},
			ctxWithin: callerCtx, wantErrSub: "exited with code 0", wantLogCount: 1, wantCalls: 1, maxElapsed: fastMax,
		},

		// happy paths
		"HealthyHealthcheckIsReady": {
			states:    []*container.State{healthyState()},
			ctxWithin: callerCtx, wantReady: true, wantLogCount: 0, wantCalls: 1, maxElapsed: fastMax,
		},
		"ProbeLessRunningIsReadyAfterStablePolls": {
			states:    []*container.State{runningNoProbeState()},
			ctxWithin: callerCtx, wantReady: true, wantLogCount: 0, wantCalls: stablePolls, maxElapsed: fastMax,
		},
		"ReadyURLReadyIsReady": {
			states: []*container.State{runningNoProbeState()}, readyURLStatus: http.StatusOK,
			ctxWithin: callerCtx, wantReady: true, wantLogCount: 0, wantCalls: 1, maxElapsed: fastMax,
		},

		// still starting or probe unhealthy: keep waiting
		"StartingHealthcheckWaitsThenTimesOut": {
			states:    []*container.State{startingHealthState()},
			ctxWithin: waitCtx, wantErrSub: "timeout waiting", wantNoErrSub: "exited with code",
			wantLogCount: 1, maxElapsed: waitMax,
		},
		"ReadyURLNotReadyWaitsThenTimesOut": {
			states: []*container.State{runningNoProbeState()}, readyURLStatus: http.StatusServiceUnavailable,
			ctxWithin: waitCtx, wantErrSub: "timeout waiting", wantNoErrSub: "exited with code",
			wantLogCount: 1, maxElapsed: waitMax,
		},
		"NilStateWaitsThenTimesOut": {
			states:    []*container.State{nil},
			ctxWithin: waitCtx, wantErrSub: "timeout waiting", wantNoErrSub: "exited with code",
			wantLogCount: 1, maxElapsed: waitMax,
		},
		"CreatedStateWaitsThenTimesOut": {
			states:    []*container.State{createdState()},
			ctxWithin: waitCtx, wantErrSub: "timeout waiting", wantNoErrSub: "exited with code",
			wantLogCount: 1, maxElapsed: waitMax,
		},

		// inspect errors propagate immediately, without the debug dump
		"InspectErrorPropagates": {
			inspectFn: func() (client.ContainerInspectResult, error) {
				return client.ContainerInspectResult{}, errors.New("daemon unreachable")
			},
			ctxWithin: callerCtx, wantErrSub: "daemon unreachable", wantNoErrSub: "timeout waiting",
			wantLogCount: 0, wantCalls: 1, maxElapsed: fastMax,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeContainerClient{
				states: tc.states, restartCounts: tc.restartCounts, inspectFn: tc.inspectFn,
			}
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
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErrSub)
				if tc.wantNoErrSub != "" {
					assert.NotContains(t, err.Error(), tc.wantNoErrSub)
				}
			}
			require.Len(t, logged, tc.wantLogCount, "logDebug call count")
			if len(logged) > 0 {
				assert.Equal(t, svcName, logged[0])
			}
			if tc.wantCalls > 0 {
				assert.Equal(t, tc.wantCalls, fake.calls, "inspect call count")
			}
			assert.Less(t, elapsed, tc.maxElapsed)
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
