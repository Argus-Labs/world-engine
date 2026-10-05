// Package debug drives a Cardinal shard's tick controls over DebugService.
// It lives here rather than in the debugger command because the log viewer runs
// the same operations from its hotkeys — one definition means a hotkey and
// `world debug <op>` cannot diverge.
package debug

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/rotisserie/eris"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

// Action applies one debug operation to an instance.
type Action func(
	ctx context.Context,
	client cardinalv1connect.DebugServiceClient,
	instanceID string,
) (string, error)

// ClientTimeout bounds a single debug RPC.
const ClientTimeout = 30 * time.Second

// NewClient dials a shard instance's DebugService. It is unauthenticated in
// dev, so there is no sign-in step.
func NewClient(url string) cardinalv1connect.DebugServiceClient {
	return cardinalv1connect.NewDebugServiceClient(&http.Client{Timeout: ClientTimeout}, url)
}

// Pause stops an instance from ticking.
func Pause(
	ctx context.Context, client cardinalv1connect.DebugServiceClient, instanceID string,
) (string, error) {
	resp, err := client.Pause(ctx, connect.NewRequest(&cardinalv1.PauseRequest{}))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Paused instance %q at tick %d", instanceID, resp.Msg.GetTickHeight()), nil
}

// Resume starts a paused instance ticking again.
func Resume(
	ctx context.Context, client cardinalv1connect.DebugServiceClient, instanceID string,
) (string, error) {
	if _, err := client.Resume(ctx, connect.NewRequest(&cardinalv1.ResumeRequest{})); err != nil {
		return "", err
	}
	return fmt.Sprintf("Resumed instance %q", instanceID), nil
}

// Step advances an instance by one tick, pausing it first if necessary.
func Step(
	ctx context.Context, client cardinalv1connect.DebugServiceClient, instanceID string,
) (string, error) {
	var resp *connect.Response[cardinalv1.StepResponse]
	autoPaused, err := runWithAutoPause(ctx, client, instanceID, func() error {
		var err error
		resp, err = client.Step(ctx, connect.NewRequest(&cardinalv1.StepRequest{}))
		return err
	})
	if err != nil {
		if autoPaused {
			return "", eris.Wrapf(err, "instance %q was paused, but step failed", instanceID)
		}
		return "", err
	}
	if autoPaused {
		return fmt.Sprintf("Paused and stepped instance %q to tick %d", instanceID, resp.Msg.GetTickHeight()), nil
	}
	return fmt.Sprintf("Stepped instance %q to tick %d", instanceID, resp.Msg.GetTickHeight()), nil
}

// Reset clears an instance, pausing it first if necessary.
func Reset(
	ctx context.Context, client cardinalv1connect.DebugServiceClient, instanceID string,
) (string, error) {
	autoPaused, err := runWithAutoPause(ctx, client, instanceID, func() error {
		_, err := client.Reset(ctx, connect.NewRequest(&cardinalv1.ResetRequest{}))
		return err
	})
	if err != nil {
		if autoPaused {
			return "", eris.Wrapf(err, "instance %q was paused, but reset failed", instanceID)
		}
		return "", err
	}
	if autoPaused {
		return fmt.Sprintf("Paused and reset instance %q", instanceID), nil
	}
	return fmt.Sprintf("Reset instance %q", instanceID), nil
}

// runWithAutoPause retries action once after a failed-precondition response,
// pausing the instance first.
func runWithAutoPause(
	ctx context.Context,
	client cardinalv1connect.DebugServiceClient,
	instanceID string,
	action func() error,
) (bool, error) {
	err := action()
	if err == nil {
		return false, nil
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		return false, err
	}
	if _, err := client.Pause(ctx, connect.NewRequest(&cardinalv1.PauseRequest{})); err != nil {
		return false, eris.Wrapf(err, "pause instance %q first", instanceID)
	}
	return true, action()
}
