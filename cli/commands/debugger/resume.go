package debugger

import (
	"context"

	"github.com/argus-labs/world-engine/cli/internal/debug"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
)

type ResumeCmd struct {
	Instances []string `name:"shards" aliases:"shard-id,instances" help:"Shard IDs to resume, e.g. game or game-2. Repeatable or comma-separated; resumes every shard if omitted."`
}

func (c *ResumeCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("resume-cardinal-command", map[string]any{
		"instances": c.Instances,
	})

	return runDebugAction(ctx, c.Instances, "resume", debug.Resume)
}
