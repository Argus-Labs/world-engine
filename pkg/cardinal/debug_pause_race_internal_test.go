package cardinal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

// This file reproduces the concurrent Pause/Resume/Step/Reset deadlock that existed in World.run
// when the run loop's select listened on a disjoint subset of the unbuffered control channels
// depending on isPaused. With the disjoint selects, flipping pause state (servicing one control
// send) moved the loop into the select that omitted every other queued sender, so the losing
// senders parked on their unbuffered channels forever -- leaking the handler goroutine and its HTTP
// connection. The fix collapses the two selects into one that ALWAYS listens on pause/resume/step/
// reset (gating only ticker.C on isPaused), and makes each handler's channel send/receive
// interruptible by ctx.Done() so a client that gives up (cancel/deadline/disconnect) terminates the
// request instead of parking forever. These tests drive the real World.run loop, the real
// *debugModule handlers, and the real unbuffered control channels; only an optional slowTickSystem
// widens the mid-tick race window for deterministic observation.
//
// These tests are intentionally serial (not t.Parallel): they use t.Setenv to suppress world logs,
// which is incompatible with t.Parallel, matching the convention used by the DST/E2E harnesses.

// slowTickSystem is an update system whose Run sleeps for a configurable duration, widening the
// window in which the run loop is inside w.Tick and therefore not parked in its control select. It
// introduces no defect of its own: it only makes the mid-tick race window observable.
type slowTickSystem struct {
	BaseSystemState
	delay time.Duration
}

func (s *slowTickSystem) Run() {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
}

// raceResult records one racing control RPC's outcome.
type raceResult struct {
	method string
	err    error
	height uint64
}

const (
	raceCallTimeout = 2 * time.Second // per-RPC deadline; generous vs. any legitimate tick work.
	raceWatchdog    = 5 * time.Second // test-level guard so a regression fails instead of hanging CI.
)

func raceCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), raceCallTimeout)
}

// raceSuppressWorldLogs disables world telemetry logging for this test. It must be called before
// NewWorld constructs the telemetry stack.
func raceSuppressWorldLogs(t *testing.T) {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")
}

// newPauseRaceWorld builds a real World (debug enabled) whose run loop ticks in the background with a
// slowTickSystem widening the mid-tick window. The loop's stop cancel is registered with t.Cleanup so
// callers need not manage it.
func newPauseRaceWorld(t *testing.T, tickDelay time.Duration) *World {
	t.Helper()
	debug := true
	w, err := NewWorld(WorldOptions{
		Region:              "pause-race",
		Organization:        "pause-race",
		Project:             "pause-race",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1,
		Debug:               &debug,
	})
	require.NoError(t, err)
	require.NotNil(t, w.debug)
	w.RegisterSystemV2(&slowTickSystem{delay: tickDelay})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = w.run(ctx) }()
	return w
}

// Direct, in-process control RPC helpers (exercise the real *debugModule handler methods and the
// real unbuffered control channels against the real loop).
func racePause(ctx context.Context, w *World) error {
	_, err := w.debug.Pause(ctx, connect.NewRequest(&cardinalv1.PauseRequest{}))
	return err
}

func raceResume(ctx context.Context, w *World) error {
	_, err := w.debug.Resume(ctx, connect.NewRequest(&cardinalv1.ResumeRequest{}))
	return err
}

func raceStep(ctx context.Context, w *World) (uint64, error) {
	resp, err := w.debug.Step(ctx, connect.NewRequest(&cardinalv1.StepRequest{}))
	if err != nil {
		return 0, err
	}
	return resp.Msg.GetTickHeight(), nil
}

// racePauseSync pauses the world synchronously, failing the test if Pause does not return promptly --
// which would itself indicate a deadlock before the race even opens.
func racePauseSync(t *testing.T, w *World) {
	t.Helper()
	ctx, cancel := raceCtx()
	defer cancel()
	require.NoError(t, racePause(ctx, w))
}

func raceStepResult(w *World) raceResult {
	ctx, cancel := raceCtx()
	defer cancel()
	h, err := raceStep(ctx, w)
	return raceResult{method: "Step", err: err, height: h}
}

func raceResumeResult(w *World) raceResult {
	ctx, cancel := raceCtx()
	defer cancel()
	return raceResult{method: "Resume", err: raceResume(ctx, w)}
}

// awaitRaceResults waits for want results, failing the test if the watchdog elapses first. A
// partial result set is exactly the deadlock signature: fewer than want handlers can return because
// their unbuffered control-channel sends are orphaned by a run-loop state transition.
func awaitRaceResults(t *testing.T, results <-chan raceResult, want int) []raceResult {
	t.Helper()
	deadline := time.NewTimer(raceWatchdog)
	defer deadline.Stop()
	got := make([]raceResult, 0, want)
	for len(got) < want {
		select {
		case r := <-results:
			got = append(got, r)
		case <-deadline.C:
			for _, r := range got {
				t.Logf("returned: method=%s err=%v height=%d", r.method, r.err, r.height)
			}
			t.Fatalf("deadlock reproduced: only %d/%d handlers returned", len(got), want)
		}
	}
	return got
}

func requireNoRaceErrors(t *testing.T, results []raceResult) {
	t.Helper()
	for _, r := range results {
		require.NoErrorf(t, r.err, "%s returned an error", r.method)
	}
}

// TestPauseResumeStepDeadlock reproduces the original deadlock: the world is paused, a lead Step
// holds the loop mid-tick, then 3 Steps and 1 Resume race through a barrier. With the disjoint
// select, servicing the Resume flipped isPaused and the remaining Step senders were orphaned on the
// unbuffered stepCh forever (the loop's running-state select had no stepCh case). With the single
// select fix, every control channel is always selectable, so all five handlers return cleanly.
func TestPauseResumeStepDeadlock(t *testing.T) {
	raceSuppressWorldLogs(t)
	const slowTick = 50 * time.Millisecond
	w := newPauseRaceWorld(t, slowTick)
	racePauseSync(t, w)

	const total = 5 // 1 lead Step + 3 Steps + 1 Resume
	results := make(chan raceResult, total)

	// Lead Step: its slow Tick holds the loop out of any select for ~slowTick.
	go func() { results <- raceStepResult(w) }()
	time.Sleep(slowTick / 2) // wait until the lead is mid-tick

	// Release 3 Steps + 1 Resume simultaneously so several senders queue before the loop re-enters
	// its select.
	release := make(chan struct{})
	for range 3 {
		go func() {
			<-release
			results <- raceStepResult(w)
		}()
	}
	go func() {
		<-release
		results <- raceResumeResult(w)
	}()
	close(release)

	got := awaitRaceResults(t, results, total)
	requireNoRaceErrors(t, got)
}

// TestDoubleResumeDeadlock reproduces the double-Resume deadlock: the world is paused, a lead Step
// holds the loop mid-tick, then 3 Resumes race. With the disjoint select, the first Resume flipped
// isPaused and the remaining two Resume senders were orphaned on the unbuffered resumeCh forever
// (the running-state select had no resumeCh case). With the single select fix all four return.
func TestDoubleResumeDeadlock(t *testing.T) {
	raceSuppressWorldLogs(t)
	const slowTick = 50 * time.Millisecond
	w := newPauseRaceWorld(t, slowTick)
	racePauseSync(t, w)

	const total = 4 // 1 lead Step + 3 Resumes
	results := make(chan raceResult, total)

	go func() { results <- raceStepResult(w) }()
	time.Sleep(slowTick / 2)

	release := make(chan struct{})
	for range 3 {
		go func() {
			<-release
			results <- raceResumeResult(w)
		}()
	}
	close(release)

	got := awaitRaceResults(t, results, total)
	requireNoRaceErrors(t, got)
}

// TestDoubleResumeDeadlockFrequency runs the double-Resume race across realistic per-tick durations
// with a FRESH world per attempt (so leaked goroutines from a deadlock do not contaminate later
// samples). It asserts zero deadlocks across the sweep: with the fix the unbuffered control sends
// are always selectable, so every attempt completes promptly. With the disjoint-select bug the
// deadlock rate at these delays is 40-96% (see the bug report), so this test fails fast on a
// regression.
func TestDoubleResumeDeadlockFrequency(t *testing.T) {
	raceSuppressWorldLogs(t)
	delays := []time.Duration{time.Millisecond, 2 * time.Millisecond}
	const attempts = 10
	const attemptTimeout = 400 * time.Millisecond

	for _, delay := range delays {
		deadlocks := 0
		for range attempts {
			if attemptResumeRace(t, delay, attemptTimeout) {
				deadlocks++
			}
		}
		require.Equalf(t, 0, deadlocks,
			"deadlock at tick_delay=%s: %d/%d attempts deadlocked", delay, deadlocks, attempts)
	}
}

// attemptResumeRace runs a single double-Resume race against a fresh world and reports whether the
// attempt deadlocked (some handlers failed to return cleanly within attemptTimeout).
func attemptResumeRace(t *testing.T, tickDelay, attemptTimeout time.Duration) bool {
	w := newPauseRaceWorld(t, tickDelay)

	pauseCtx, cancelPause := raceCtx()
	defer cancelPause()
	if err := racePause(pauseCtx, w); err != nil {
		t.Logf("attempt: pause failed: %v", err)
		return true
	}

	const total = 4 // 1 lead Step + 3 Resumes
	results := make(chan raceResult, total)

	go func() { results <- raceStepResult(w) }()
	time.Sleep(tickDelay / 2) // wait until the lead is mid-tick
	for range 3 {
		go func() { results <- raceResumeResult(w) }()
	}

	// A deadlock is a handler that never returns (its unbuffered control send is orphaned by a
	// run-loop state transition). A handler that returns with failed_precondition lost the benign
	// isPaused precondition race (it checked after a winner already flipped the state) and is NOT a
	// deadlock -- it returned cleanly. So count any result, error or not, as "returned"; only a
	// no-return within attemptTimeout is a deadlock.
	deadline := time.NewTimer(attemptTimeout)
	defer deadline.Stop()
	got := 0
	for got < total {
		select {
		case r := <-results:
			if r.err != nil {
				t.Logf("attempt: %s returned (benign) error: %v", r.method, r.err)
			}
			got++
		case <-deadline.C:
			t.Logf("attempt: deadlock at tick_delay=%s: only %d/%d returned", tickDelay, got, total)
			return true
		}
	}
	return false
}

// TestDoubleResumeDeadlockOverHTTP drives the real mounted DebugService over real HTTP hops via the
// generated connect client (the same client stub external tools use). The deadlock is reachable
// over HTTP, not only via in-process function calls; this asserts the fix closes that path too.
func TestDoubleResumeDeadlockOverHTTP(t *testing.T) {
	raceSuppressWorldLogs(t)
	const slowTick = 50 * time.Millisecond
	w := newPauseRaceWorld(t, slowTick)

	// Mount the real DebugService exactly as production does (minus interceptors, which do not affect
	// the control-channel handshake) and drive it over HTTP.
	mux := http.NewServeMux()
	debugPath, debugHandler := cardinalv1connect.NewDebugServiceHandler(w.debug)
	mux.Handle(debugPath, debugHandler)
	server := httptest.NewServer(mux)
	// Close the listener and client connections without blocking cleanup on in-flight handlers: under
	// a regression a handler may park forever on an unbuffered control channel, and httptest.Server.Close
	// blocks until in-flight handlers return. Run it in a goroutine so the test fails fast instead of
	// hanging the process; a correct fix never parks, so Close completes immediately and nothing leaks.
	t.Cleanup(func() {
		server.CloseClientConnections()
		go server.Close()
	})
	client := cardinalv1connect.NewDebugServiceClient(server.Client(), server.URL)

	// Pause over HTTP first.
	pauseCtx, cancelPause := raceCtx()
	defer cancelPause()
	_, err := client.Pause(pauseCtx, connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)

	httpStep := func() raceResult {
		ctx, cancel := raceCtx()
		defer cancel()
		resp, err := client.Step(ctx, connect.NewRequest(&cardinalv1.StepRequest{}))
		h := uint64(0)
		if err == nil {
			h = resp.Msg.GetTickHeight()
		}
		return raceResult{method: "Step", err: err, height: h}
	}
	httpResume := func() raceResult {
		ctx, cancel := raceCtx()
		defer cancel()
		_, err := client.Resume(ctx, connect.NewRequest(&cardinalv1.ResumeRequest{}))
		return raceResult{method: "Resume", err: err}
	}

	const total = 4 // 1 lead Step + 3 Resumes
	results := make(chan raceResult, total)

	go func() { results <- httpStep() }()
	time.Sleep(slowTick / 2)

	release := make(chan struct{})
	for range 3 {
		go func() {
			<-release
			results <- httpResume()
		}()
	}
	close(release)

	got := awaitRaceResults(t, results, total)
	requireNoRaceErrors(t, got)
}

// TestControlRPCCtxCancelReturnsCleanly verifies the handler-side half of the fix: each control
// handler must interrupt its unbuffered channel send/receive on ctx.Done() so a client that cancels
// (cancel/deadline/disconnect) terminates the request instead of parking forever. A lead Step holds
// the loop mid-tick so the control send parks; canceling the client ctx must surface a clean
// CodeCanceled error promptly.
func TestControlRPCCtxCancelReturnsCleanly(t *testing.T) {
	raceSuppressWorldLogs(t)
	cases := []struct {
		name string
		call func(context.Context, *World) error
	}{
		{"Resume", raceResume},
		{"Step", func(ctx context.Context, w *World) error { _, err := raceStep(ctx, w); return err }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const slowTick = 100 * time.Millisecond // wide window so the parked send reliably precedes cancel
			w := newPauseRaceWorld(t, slowTick)
			racePauseSync(t, w)

			// Lead Step opens the mid-tick window so the control send below parks on its unbuffered
			// channel. Its result is intentionally discarded.
			go func() { _ = raceStepResult(w) }()
			time.Sleep(slowTick / 2) // wait until the lead is mid-tick

			ctx, cancel := context.WithCancel(context.Background())
			errCh := make(chan error, 1)
			go func() { errCh <- tc.call(ctx, w) }()

			// Cancel well before the lead's slow tick finishes, while the loop is still mid-tick and the
			// handler is parked on its unbuffered send.
			time.Sleep(slowTick / 8)
			cancel()

			select {
			case err := <-errCh:
				require.Error(t, err, "canceled control RPC should return an error")
				assert.Equal(t, connect.CodeCanceled, connect.CodeOf(err),
					"canceled control RPC should surface as CodeCanceled")
			case <-time.After(raceCallTimeout):
				t.Fatal("control RPC did not return after ctx cancel; handler is not ctx-aware")
			}
		})
	}
}
