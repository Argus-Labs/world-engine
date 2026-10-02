package cardinal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	cardinalv1connect "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

// -------------------------------------------------------------------------------------------------
// Test helpers
// -------------------------------------------------------------------------------------------------

// newBareDebugModule creates a debugModule with only a tickControl (no world) for testing
// handler-level behavior in isolation. The caller sets the pause state via d.control.isPaused.
func newBareDebugModule(paused bool) *debugModule {
	d := &debugModule{control: newTickControl()}
	d.control.isPaused.Store(paused)
	return d
}

// newDebugControlWorld creates a minimal *World with debug enabled, suitable for exercising
// the real World.run loop in tests. It does NOT call w.world.Init() — that is done by run.
func newDebugControlWorld(t *testing.T) *World {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")

	debug := true
	pprof := false
	w, err := NewWorld(WorldOptions{
		Region:              "dbg-ctrl",
		Organization:        "dbg-ctrl",
		Project:             "dbg-ctrl",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        5,
		Debug:               &debug,
		Pprof:               &pprof,
	})
	require.NoError(t, err)
	require.NotNil(t, w.debug)
	return w
}

// startRunLoop starts w.run in a goroutine and returns a cancel function that stops it.
// It also registers cleanup to guarantee the goroutine exits before the test ends.
func startRunLoop(t *testing.T, w *World) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("w.run did not exit within 5s after cancel")
		}
	})
	return cancel
}

// recvOrTimeout reads from a channel with a 1s timeout, failing the test on timeout.
// Returns the value and the ok flag (false if the channel was closed).
func recvOrTimeout[T any](t *testing.T, ch <-chan T, msg string) (T, bool) {
	t.Helper()
	select {
	case v, ok := <-ch:
		return v, ok
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting: %s", msg)
		return *new(T), false
	}
}

// waitForResume polls isPaused until it returns false, failing the test on timeout.
// The Resume handler returns as soon as the loop receives the resumeCh signal, but the
// loop has not yet executed setPaused(false). This helper bridges that window.
func waitForResume(t *testing.T, d *debugModule) {
	t.Helper()
	require.Eventually(t, func() bool { return !d.isPaused() }, time.Second, 5*time.Millisecond,
		"world should not be paused after Resume")
}

// debugE2EServer sets up a real *World with Debug enabled, mounts the DebugService on an
// httptest.Server, and optionally starts the tick loop. It returns the server and a
// ConnectRPC client for making real HTTP calls to the debug service.
type debugE2EServer struct {
	world  *World
	server *httptest.Server
	client cardinalv1connect.DebugServiceClient
}

func newDebugE2EServer(t *testing.T, startLoop bool) *debugE2EServer {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")

	debug := true
	pprof := false
	w, err := NewWorld(WorldOptions{
		Region:              "e2e-ctrl",
		Organization:        "e2e-ctrl",
		Project:             "e2e-ctrl",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        5,
		Debug:               &debug,
		Pprof:               &pprof,
	})
	require.NoError(t, err)
	require.NotNil(t, w.debug)

	mux := http.NewServeMux()
	require.NoError(t, w.service.mountDebugService(mux))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := cardinalv1connect.NewDebugServiceClient(server.Client(), server.URL)

	if startLoop {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			_ = w.run(ctx)
			close(done)
		}()
		t.Cleanup(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("w.run did not exit within 5s after cancel")
			}
		})
	}

	return &debugE2EServer{world: w, server: server, client: client}
}

// -------------------------------------------------------------------------------------------------
// Handler context-cancellation (deterministic, no loop)
//
// Each handler must return connect.CodeCanceled when its context expires, even if no loop
// is reading the control channel. Before the fix, the bare unbuffered send blocked past the
// deadline with no select on ctx.Done().
// -------------------------------------------------------------------------------------------------

func testHandlerRespectsContextDeadline(
	t *testing.T,
	call func(ctx context.Context) error,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := call(ctx)
	elapsed := time.Since(start)

	require.Error(t, err, "handler must return an error when context expires")
	require.Equal(t, connect.CodeCanceled, connect.CodeOf(err),
		"handler must return CodeCanceled, not hang")
	assert.Less(t, elapsed, 2*time.Second,
		"handler must return shortly after the context deadline, not hang indefinitely")
}

func TestPauseRespectsContextDeadline(t *testing.T) {
	testHandlerRespectsContextDeadline(t, func(ctx context.Context) error {
		_, err := newBareDebugModule(false).Pause(ctx, nil)
		return err
	})
}

func TestResumeRespectsContextDeadline(t *testing.T) {
	testHandlerRespectsContextDeadline(t, func(ctx context.Context) error {
		_, err := newBareDebugModule(true).Resume(ctx, nil)
		return err
	})
}

func TestStepRespectsContextDeadline(t *testing.T) {
	testHandlerRespectsContextDeadline(t, func(ctx context.Context) error {
		_, err := newBareDebugModule(true).Step(ctx, nil)
		return err
	})
}

func TestResetRespectsContextDeadline(t *testing.T) {
	testHandlerRespectsContextDeadline(t, func(ctx context.Context) error {
		_, err := newBareDebugModule(true).Reset(ctx, nil)
		return err
	})
}

func testHandlerRespectsContextCancel(
	t *testing.T,
	call func(ctx context.Context) error,
) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- call(ctx) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.Error(t, err)
		require.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after context cancel")
	}
}

func TestPauseRespectsContextCancel(t *testing.T) {
	d := newBareDebugModule(false)
	testHandlerRespectsContextCancel(t, func(ctx context.Context) error {
		_, err := d.Pause(ctx, nil)
		return err
	})
}

func TestStepRespectsContextCancel(t *testing.T) {
	d := newBareDebugModule(true)
	testHandlerRespectsContextCancel(t, func(ctx context.Context) error {
		_, err := d.Step(ctx, nil)
		return err
	})
}

func TestResumeRespectsContextCancel(t *testing.T) {
	d := newBareDebugModule(true)
	testHandlerRespectsContextCancel(t, func(ctx context.Context) error {
		_, err := d.Resume(ctx, nil)
		return err
	})
}

func TestResetRespectsContextCancel(t *testing.T) {
	d := newBareDebugModule(true)
	testHandlerRespectsContextCancel(t, func(ctx context.Context) error {
		_, err := d.Reset(ctx, nil)
		return err
	})
}

// -------------------------------------------------------------------------------------------------
// Handler NACK handling (deterministic)
//
// The loop NACKs a stale Step/Reset by closing the reply channel. The handler must detect
// the closed channel (ok == false) and return FailedPrecondition — not hang, not panic, and
// not return a zero-value success.
// -------------------------------------------------------------------------------------------------

func TestStepHandlerNACKReturnsFailedPrecondition(t *testing.T) {
	d := newBareDebugModule(true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- func() error { _, err := d.Step(ctx, nil); return err }() }()

	replyCh, _ := recvOrTimeout(t, d.control.stepCh, "handler should send on stepCh")

	d.control.isPaused.Store(false)
	close(replyCh) // NACK

	select {
	case err := <-errCh:
		require.Error(t, err)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err),
			"Step handler must return FailedPrecondition on NACK")
	case <-time.After(2 * time.Second):
		t.Fatal("Step handler did not return after NACK")
	}
}

func TestResetHandlerNACKReturnsFailedPrecondition(t *testing.T) {
	d := newBareDebugModule(true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- func() error { _, err := d.Reset(ctx, nil); return err }() }()

	replyCh, _ := recvOrTimeout(t, d.control.resetCh, "handler should send on resetCh")

	d.control.isPaused.Store(false)
	close(replyCh) // NACK

	select {
	case err := <-errCh:
		require.Error(t, err)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err),
			"Reset handler must return FailedPrecondition on NACK")
	case <-time.After(2 * time.Second):
		t.Fatal("Reset handler did not return after NACK")
	}
}

// -------------------------------------------------------------------------------------------------
// Precondition checks (deterministic, no loop)
// -------------------------------------------------------------------------------------------------

func TestPauseFailsWhenAlreadyPaused(t *testing.T) {
	d := newBareDebugModule(true)
	_, err := d.Pause(context.Background(), nil)
	require.Error(t, err)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestResumeFailsWhenNotPaused(t *testing.T) {
	d := newBareDebugModule(false)
	_, err := d.Resume(context.Background(), nil)
	require.Error(t, err)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestStepFailsWhenNotPaused(t *testing.T) {
	d := newBareDebugModule(false)
	_, err := d.Step(context.Background(), nil)
	require.Error(t, err)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestResetFailsWhenNotPaused(t *testing.T) {
	d := newBareDebugModule(false)
	_, err := d.Reset(context.Background(), nil)
	require.Error(t, err)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

// -------------------------------------------------------------------------------------------------
// Real World.run loop integration
//
// Note: w.currentTick.height is never read directly while the loop is running (it is
// written by the loop goroutine with no synchronization). Tick heights are obtained from
// RPC responses, which synchronize via the reply channel.
// -------------------------------------------------------------------------------------------------

func TestControlHappyPath(t *testing.T) {
	w := newDebugControlWorld(t)
	stop := startRunLoop(t, w)
	defer stop()

	time.Sleep(50 * time.Millisecond)
	pauseResp, err := w.debug.Pause(context.Background(), connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)
	pauseH := pauseResp.Msg.GetTickHeight()
	require.Positive(t, pauseH, "world should have ticked before pause")
	require.True(t, w.debug.isPaused())

	step1, err := w.debug.Step(context.Background(), connect.NewRequest(&cardinalv1.StepRequest{}))
	require.NoError(t, err)
	require.Equal(t, pauseH+1, step1.Msg.GetTickHeight())

	step2, err := w.debug.Step(context.Background(), connect.NewRequest(&cardinalv1.StepRequest{}))
	require.NoError(t, err)
	require.Equal(t, pauseH+2, step2.Msg.GetTickHeight())

	// No ticks while paused: wait beyond tick intervals, then step and verify
	// the height is exactly pauseH+3 (no ticker ticks fired during the wait).
	time.Sleep(50 * time.Millisecond)
	step3, err := w.debug.Step(context.Background(), connect.NewRequest(&cardinalv1.StepRequest{}))
	require.NoError(t, err)
	require.Equal(t, pauseH+3, step3.Msg.GetTickHeight(),
		"no ticker ticks should fire while paused")

	_, err = w.debug.Resume(context.Background(), connect.NewRequest(&cardinalv1.ResumeRequest{}))
	require.NoError(t, err)
	waitForResume(t, w.debug)

	time.Sleep(50 * time.Millisecond)
	pause2Resp, err := w.debug.Pause(context.Background(), connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)
	require.Greater(t, pause2Resp.Msg.GetTickHeight(), pauseH+3, "world should tick after resume")
}

func TestNoTicksWhilePaused(t *testing.T) {
	w := newDebugControlWorld(t)
	stop := startRunLoop(t, w)
	defer stop()

	time.Sleep(50 * time.Millisecond)
	pauseResp, err := w.debug.Pause(context.Background(), connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)
	pauseH := pauseResp.Msg.GetTickHeight()
	require.True(t, w.debug.isPaused())

	time.Sleep(100 * time.Millisecond)

	stepResp, err := w.debug.Step(context.Background(), connect.NewRequest(&cardinalv1.StepRequest{}))
	require.NoError(t, err)
	require.Equal(t, pauseH+1, stepResp.Msg.GetTickHeight(),
		"tick height must advance by exactly one step while paused, not by ticker ticks")
}

func TestStepNACKWhileRunning(t *testing.T) {
	w := newDebugControlWorld(t)
	stop := startRunLoop(t, w)
	defer stop()

	time.Sleep(20 * time.Millisecond)

	replyCh := make(chan uint64, 1)
	select {
	case w.debug.control.stepCh <- replyCh:
	case <-time.After(time.Second):
		t.Fatal("single-select loop must receive stepCh send while running (not hang)")
	}

	_, ok := recvOrTimeout(t, replyCh, "should receive NACK from loop")
	require.False(t, ok, "loop must NACK stale step by closing replyCh, not send a height")
}

func TestResetNACKWhileRunning(t *testing.T) {
	w := newDebugControlWorld(t)
	stop := startRunLoop(t, w)
	defer stop()

	time.Sleep(20 * time.Millisecond)

	replyCh := make(chan struct{}, 1)
	select {
	case w.debug.control.resetCh <- replyCh:
	case <-time.After(time.Second):
		t.Fatal("single-select loop must receive resetCh send while running (not hang)")
	}

	_, ok := recvOrTimeout(t, replyCh, "should receive NACK from loop")
	require.False(t, ok, "loop must NACK stale reset by closing replyCh")
}

// TestStaleStepDoesNotFireLateTick is the end-to-end reproduction of the primary harm:
// a leaked Step must not fire a late tick after the world is re-paused. With the fix, the
// stale Step send is NACKed immediately while the world is running; subsequent re-pause
// does not trigger an extra tick.
func TestStaleStepDoesNotFireLateTick(t *testing.T) {
	w := newDebugControlWorld(t)
	stop := startRunLoop(t, w)
	defer stop()

	time.Sleep(50 * time.Millisecond)
	pauseResp, err := w.debug.Pause(context.Background(), connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)
	pauseH := pauseResp.Msg.GetTickHeight()
	require.Positive(t, pauseH)

	_, err = w.debug.Resume(context.Background(), connect.NewRequest(&cardinalv1.ResumeRequest{}))
	require.NoError(t, err)
	waitForResume(t, w.debug)

	staleReply := make(chan uint64, 1)
	select {
	case w.debug.control.stepCh <- staleReply:
	case <-time.After(time.Second):
		t.Fatal("single-select loop must receive stale stepCh send while running")
	}

	_, ok := recvOrTimeout(t, staleReply, "stale step NACK")
	require.False(t, ok, "stale step must be NACKed, not fulfilled")

	time.Sleep(50 * time.Millisecond)

	pause2Resp, err := w.debug.Pause(context.Background(), connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)
	pause2H := pause2Resp.Msg.GetTickHeight()
	require.True(t, w.debug.isPaused())
	require.Greater(t, pause2H, pauseH, "ticker.C should have advanced the world while running")

	stepResp, err := w.debug.Step(context.Background(), connect.NewRequest(&cardinalv1.StepRequest{}))
	require.NoError(t, err)
	require.Equal(t, pause2H+1, stepResp.Msg.GetTickHeight(),
		"explicit Step should add exactly one tick; stale step must not have fired late")
}

func TestConcurrentControlsNoLeak(t *testing.T) {
	w := newDebugControlWorld(t)
	stop := startRunLoop(t, w)
	defer stop()

	_, err := w.debug.Pause(context.Background(), connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)

	const burst = 50
	var wg sync.WaitGroup
	wg.Add(burst)

	for range burst {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			_, _ = w.debug.Step(ctx, connect.NewRequest(&cardinalv1.StepRequest{}))
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent Step calls did not all return within 3s — goroutine leak")
	}
}

// -------------------------------------------------------------------------------------------------
// E2E tests (real HTTP ConnectRPC transport)
// -------------------------------------------------------------------------------------------------

func TestE2EHappyPath(t *testing.T) {
	e2e := newDebugE2EServer(t, true)

	time.Sleep(50 * time.Millisecond)

	pauseResp, err := e2e.client.Pause(context.Background(),
		connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err, "Pause RPC should succeed")
	pauseH := pauseResp.Msg.GetTickHeight()
	require.Positive(t, pauseH)

	stateResp, err := e2e.client.GetState(context.Background(),
		connect.NewRequest(&cardinalv1.GetStateRequest{}))
	require.NoError(t, err)
	require.True(t, stateResp.Msg.GetIsPaused(), "GetState must report IsPaused=true after Pause")

	stepResp, err := e2e.client.Step(context.Background(),
		connect.NewRequest(&cardinalv1.StepRequest{}))
	require.NoError(t, err)
	require.Equal(t, pauseH+1, stepResp.Msg.GetTickHeight())

	_, err = e2e.client.Resume(context.Background(),
		connect.NewRequest(&cardinalv1.ResumeRequest{}))
	require.NoError(t, err)
	require.Eventually(t, func() bool { return !e2e.world.debug.isPaused() },
		time.Second, 5*time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	pause2Resp, err := e2e.client.Pause(context.Background(),
		connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)
	require.Greater(t, pause2Resp.Msg.GetTickHeight(), pauseH+1,
		"world should have ticked after Resume")
}

func TestE2ECancellationAllHandlers(t *testing.T) {
	e2e := newDebugE2EServer(t, false)

	tests := []struct {
		name  string
		setup func()
		call  func(ctx context.Context) error
	}{
		{"Pause", func() {}, func(ctx context.Context) error {
			_, err := e2e.client.Pause(ctx, connect.NewRequest(&cardinalv1.PauseRequest{}))
			return err
		}},
		{"Resume", func() { e2e.world.debug.control.isPaused.Store(true) }, func(ctx context.Context) error {
			_, err := e2e.client.Resume(ctx, connect.NewRequest(&cardinalv1.ResumeRequest{}))
			e2e.world.debug.control.isPaused.Store(false)
			return err
		}},
		{"Step", func() { e2e.world.debug.control.isPaused.Store(true) }, func(ctx context.Context) error {
			_, err := e2e.client.Step(ctx, connect.NewRequest(&cardinalv1.StepRequest{}))
			e2e.world.debug.control.isPaused.Store(false)
			return err
		}},
		{"Reset", func() { e2e.world.debug.control.isPaused.Store(true) }, func(ctx context.Context) error {
			_, err := e2e.client.Reset(ctx, connect.NewRequest(&cardinalv1.ResetRequest{}))
			e2e.world.debug.control.isPaused.Store(false)
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			start := time.Now()
			err := tc.call(ctx)
			elapsed := time.Since(start)

			require.Error(t, err)
			code := connect.CodeOf(err)
			assert.True(t, code == connect.CodeCanceled || code == connect.CodeDeadlineExceeded,
				"%s must return Canceled or DeadlineExceeded, got %s", tc.name, code)
			assert.Less(t, elapsed, 2*time.Second,
				"%s must return shortly after the context deadline", tc.name)
		})
	}
}

func TestE2ENoLateTick(t *testing.T) {
	e2e := newDebugE2EServer(t, true)

	time.Sleep(50 * time.Millisecond)

	pauseResp, err := e2e.client.Pause(context.Background(),
		connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)
	pauseH := pauseResp.Msg.GetTickHeight()

	_, err = e2e.client.Resume(context.Background(),
		connect.NewRequest(&cardinalv1.ResumeRequest{}))
	require.NoError(t, err)
	require.Eventually(t, func() bool { return !e2e.world.debug.isPaused() },
		time.Second, 5*time.Millisecond)

	_, err = e2e.client.Step(context.Background(),
		connect.NewRequest(&cardinalv1.StepRequest{}))
	require.Error(t, err)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err),
		"Step while running must return FailedPrecondition")

	time.Sleep(50 * time.Millisecond)

	pause2Resp, err := e2e.client.Pause(context.Background(),
		connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)
	pause2H := pause2Resp.Msg.GetTickHeight()
	require.Greater(t, pause2H, pauseH)

	stepResp, err := e2e.client.Step(context.Background(),
		connect.NewRequest(&cardinalv1.StepRequest{}))
	require.NoError(t, err)
	require.Equal(t, pause2H+1, stepResp.Msg.GetTickHeight(),
		"Step should add exactly one tick; no stale late tick should have fired")
}

func TestE2EDisconnect(t *testing.T) {
	e2e := newDebugE2EServer(t, false)
	e2e.world.debug.control.isPaused.Store(true)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := e2e.client.Step(ctx, connect.NewRequest(&cardinalv1.StepRequest{}))
		errCh <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.Error(t, err)
		code := connect.CodeOf(err)
		assert.True(t, code == connect.CodeCanceled || code == connect.CodeDeadlineExceeded,
			"Step must return Canceled or DeadlineExceeded after disconnect, got %s", code)
	case <-time.After(3 * time.Second):
		t.Fatal("Step handler did not return after client disconnect — goroutine leak")
	}

	e2e.world.debug.control.isPaused.Store(false)
}
