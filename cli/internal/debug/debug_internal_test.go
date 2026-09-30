package debug

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

// fakeDebugClient records calls and can fail the first non-pause one with the
// code a running shard returns.
type fakeDebugClient struct {
	cardinalv1connect.DebugServiceClient
	calls            []string
	stepErrors       []error
	failWhileRunning bool
	paused           bool
}

func (f *fakeDebugClient) Pause(
	_ context.Context, _ *connect.Request[cardinalv1.PauseRequest],
) (*connect.Response[cardinalv1.PauseResponse], error) {
	f.calls = append(f.calls, "pause")
	f.paused = true
	return connect.NewResponse(&cardinalv1.PauseResponse{TickHeight: 7}), nil
}

func (f *fakeDebugClient) Step(
	_ context.Context, _ *connect.Request[cardinalv1.StepRequest],
) (*connect.Response[cardinalv1.StepResponse], error) {
	f.calls = append(f.calls, "step")
	if len(f.stepErrors) > 0 {
		err := f.stepErrors[0]
		f.stepErrors = f.stepErrors[1:]
		if err != nil {
			return nil, err
		}
	}
	if f.failWhileRunning && !f.paused {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world is not paused"))
	}
	return connect.NewResponse(&cardinalv1.StepResponse{TickHeight: 8}), nil
}

func (f *fakeDebugClient) Reset(
	_ context.Context, _ *connect.Request[cardinalv1.ResetRequest],
) (*connect.Response[cardinalv1.ResetResponse], error) {
	f.calls = append(f.calls, "reset")
	if f.failWhileRunning && !f.paused {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world is not paused"))
	}
	return connect.NewResponse(&cardinalv1.ResetResponse{}), nil
}

// A running shard is paused and the step retried, and the message says so.
func TestStep_AutoPausesWhenRunning(t *testing.T) {
	t.Parallel()
	fake := &fakeDebugClient{failWhileRunning: true}
	msg, err := Step(context.Background(), fake, "game")

	require.NoError(t, err)
	assert.Equal(t, []string{"step", "pause", "step"}, fake.calls)
	assert.Equal(t, `Paused and stepped instance "game" to tick 8`, msg)
}

// An already-paused shard steps directly, with no pause and no retry.
func TestStep_AlreadyPaused_DoesNotPause(t *testing.T) {
	t.Parallel()
	fake := &fakeDebugClient{failWhileRunning: true, paused: true}
	msg, err := Step(context.Background(), fake, "game")

	require.NoError(t, err)
	assert.Equal(t, []string{"step"}, fake.calls)
	assert.Equal(t, `Stepped instance "game" to tick 8`, msg)
}

func TestReset_AutoPausesWhenRunning(t *testing.T) {
	t.Parallel()
	fake := &fakeDebugClient{failWhileRunning: true}
	msg, err := Reset(context.Background(), fake, "game")

	require.NoError(t, err)
	assert.Equal(t, []string{"reset", "pause", "reset"}, fake.calls)
	assert.Equal(t, `Paused and reset instance "game"`, msg)
}

func TestReset_AlreadyPaused_DoesNotPause(t *testing.T) {
	t.Parallel()
	fake := &fakeDebugClient{failWhileRunning: true, paused: true}
	msg, err := Reset(context.Background(), fake, "game")

	require.NoError(t, err)
	assert.Equal(t, []string{"reset"}, fake.calls)
	assert.Equal(t, `Reset instance "game"`, msg)
}

func TestStep_DoesNotPauseForOtherErrors(t *testing.T) {
	t.Parallel()
	unavailable := connect.NewError(connect.CodeUnavailable, eris.New("unavailable"))
	fake := &fakeDebugClient{stepErrors: []error{unavailable}}

	_, err := Step(context.Background(), fake, "game")

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	assert.Equal(t, []string{"step"}, fake.calls)
}

func TestStep_RetryFailureReportsAutoPause(t *testing.T) {
	t.Parallel()
	notPaused := connect.NewError(connect.CodeFailedPrecondition, eris.New("world is not paused"))
	unavailable := connect.NewError(connect.CodeUnavailable, eris.New("unavailable"))
	fake := &fakeDebugClient{stepErrors: []error{notPaused, unavailable}}

	_, err := Step(context.Background(), fake, "game")

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	assert.ErrorContains(t, err, `instance "game" was paused, but step failed`)
	assert.Equal(t, []string{"step", "pause", "step"}, fake.calls)
}
