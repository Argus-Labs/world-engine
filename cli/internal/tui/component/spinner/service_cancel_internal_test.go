package spinner

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
)

// TestRun_MidRunExecCancel_ReturnsSilentError is the regression test for the Ctrl+C-during-Docker-build
// bug. exec.CommandContext, once the process has started, returns *exec.ExitError ("signal: killed") on
// cancellation — its chain never carries context.Canceled — so the old eris.Is(opErr, context.Canceled)
// gate missed this case and the noisy "signal: killed" reached main.go's print+Sentry path. The fix keys
// off the cancelled spinCtx instead. This cancels mid-run (the realistic "user waits, then gives up" path)
// and asserts the result is silenced.
func TestRun_MidRunExecCancel_ReturnsSilentError(t *testing.T) {
	// parentCtx stands in for the top-level ctx that contextWithSigterm cancels on a real Ctrl+C;
	// cancelling it cancels the spinCtx Run derives from it.
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	err := Run(parentCtx, "building (test)", func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "sleep", "5")
		go func() {
			time.Sleep(100 * time.Millisecond)
			parentCancel()
		}()
		if runErr := cmd.Run(); runErr != nil {
			return eris.Wrap(runErr, "ensure buf image")
		}
		return nil
	})

	require.Error(t, err, "mid-run cancel must surface a non-nil (now-silent) error")
	assert.True(t, errorspkg.IsSilent(err),
		"mid-run cancel of exec.CommandContext must be silent — was the original bug")
}

// TestRun_BeforeStartExecCancel_ReturnsSilentError is the regression test for the cancel-before-process-
// start path, which the old gate already caught (os/exec returns context.Canceled when the ctx is done
// before Start). The fix must keep this case silent too.
func TestRun_BeforeStartExecCancel_ReturnsSilentError(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	parentCancel()

	err := Run(parentCtx, "building (test)", func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "sleep", "5")
		if runErr := cmd.Run(); runErr != nil {
			return eris.Wrap(runErr, "ensure buf image")
		}
		return nil
	})

	require.Error(t, err)
	assert.True(t, errorspkg.IsSilent(err), "before-start cancel must remain silent")
}

// TestRun_ParentCancel_MidRunNonExecError_ReturnsSilentError confirms the fix is not specific to
// exec.ExitError: any fn failure observed after the spinCtx was cancelled (here a synthetic error raced
// with a parent cancellation) is silenced, matching the "don't flood the terminal on Ctrl+C" contract.
func TestRun_ParentCancel_MidRunNonExecError_ReturnsSilentError(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	err := Run(parentCtx, "building (test)", func(ctx context.Context) error {
		go func() {
			time.Sleep(50 * time.Millisecond)
			parentCancel()
		}()
		<-ctx.Done()
		return eris.New("synthetic failure observed after cancellation")
	})

	require.Error(t, err)
	assert.True(t, errorspkg.IsSilent(err),
		"any failure after a cancelled spinCtx must be silent")
}

// TestRun_GenuineError_NotSilent guards against over-suppression: a real failure with NO cancellation
// must still reach the user (and Sentry) — only an interrupted spinCtx triggers the silent path.
func TestRun_GenuineError_NotSilent(t *testing.T) {
	err := Run(context.Background(), "building (test)", func(ctx context.Context) error {
		return eris.New("docker build failed: simulated real failure")
	})

	require.Error(t, err)
	assert.False(t, errorspkg.IsSilent(err),
		"a genuine non-cancellation failure must NOT be silenced")
	assert.Contains(t, err.Error(), "simulated real failure")
}

// TestRun_Success_ReturnsNil confirms the happy path: a successful fn yields nil (no fabricated error)
// even though the spinCtx itself stays live throughout.
func TestRun_Success_ReturnsNil(t *testing.T) {
	err := Run(context.Background(), "building (test)", func(ctx context.Context) error {
		return nil
	})
	assert.NoError(t, err)
}

// TestRun_SpinCtxReleasedAfterComplete confirms Run does not leak a cancelled spinCtx to callers: a real
// fn that finishes successfully leaves a NON-cancelled spinCtx, so a genuine error returned by a later
// caller is not wrongly silenced. (Run derives spinCtx internally; this test exercises the success path.)
func TestRun_SpinCtxReleasedAfterComplete(t *testing.T) {
	called := false
	err := Run(context.Background(), "building (test)", func(ctx context.Context) error {
		called = true
		assert.NotNil(t, ctx)
		assert.NoError(t, ctx.Err(), "spinCtx must not be pre-cancelled on a normal run")
		return nil
	})
	assert.True(t, called)
	assert.NoError(t, err)
}

// TestExecCommandContext_Cancel_MidRun_NotContextCanceled characterizes the os/exec behaviour that
// motivates the fix: a cancel landing AFTER the process has started surfaces *exec.ExitError, not
// context.Canceled, so an eris.Is(opErr, context.Canceled) gate cannot catch it. The process is Start-ed
// before the cancel is scheduled so the test reliably exercises the mid-run path. Documented here so the
// reason the fix keys off spinCtx.Err() stays obvious.
func TestExecCommandContext_Cancel_MidRun_NotContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "sleep", "5")
	require.NoError(t, cmd.Start(), "process must be running before we cancel to exercise mid-run")
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	procErr := cmd.Wait()

	require.Error(t, procErr, "mid-run cancel SIGKILLs the subprocess")
	outer := eris.Wrap(procErr, "ensure buf image")
	assert.False(t, eris.Is(outer, context.Canceled),
		"characterization: mid-run exec cancel yields *exec.ExitError, NOT context.Canceled")
	assert.Contains(t, procErr.Error(), "killed",
		"the SIGKILL message the user would otherwise see printed")
}

// TestExecCommandContext_Cancel_BeforeStart_IsContextCanceled characterizes the only case the old gate
// caught: os/exec returns ctx.Err() (context.Canceled) when the context is already done before the
// process starts. The fix handles this case identically via spinCtx.Err().
func TestExecCommandContext_Cancel_BeforeStart_IsContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := exec.CommandContext(ctx, "sleep", "5")
	procErr := cmd.Run()

	require.Error(t, procErr)
	outer := eris.Wrap(procErr, "ensure buf image")
	assert.True(t, eris.Is(outer, context.Canceled),
		"characterization: before-start cancel yields context.Canceled (the case the old gate caught)")
}
