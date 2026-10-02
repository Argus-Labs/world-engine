package debugger

import (
	"context"

	"github.com/argus-labs/world-engine/cli/internal/debug"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
)

type ResetCmd struct {
	Instances []string `name:"shards" aliases:"shard-id,instances" help:"Shard IDs to reset, e.g. game or game-2. Repeatable or comma-separated; resets every shard if omitted."`
}

func (c *ResetCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("reset-cardinal-command", map[string]any{
		"instances": c.Instances,
	})

	return runDebugAction(ctx, c.Instances, "reset", debug.Reset)
}
