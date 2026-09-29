package debugger

import (
	"context"

	"github.com/argus-labs/world-engine/cli/internal/debug"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
)

type StepCmd struct {
	Instances []string `help:"Instance IDs to step, e.g. game or game-2. Repeatable or comma-separated; steps every instance if omitted."`
}

func (c *StepCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("step-cardinal-command", map[string]any{
		"instances": c.Instances,
	})

	return runDebugAction(ctx, c.Instances, "step", debug.Step)
}
