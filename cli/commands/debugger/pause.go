package debugger

import (
	"context"

	"github.com/argus-labs/world-engine/cli/internal/debug"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
)

type PauseCmd struct {
	Instances []string `help:"Instance IDs to pause, e.g. game or game-2. Repeatable or comma-separated; pauses every instance if omitted."`
}

func (c *PauseCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("pause-cardinal-command", map[string]any{
		"instances": c.Instances,
	})

	return runDebugAction(ctx, c.Instances, "pause", debug.Pause)
}
