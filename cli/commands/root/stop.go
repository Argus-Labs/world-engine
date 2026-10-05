package root

import (
	"context"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/pkg/cluster"
)

type StopCmd struct{}

func (c *StopCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("stop-cardinal-command", map[string]any{})

	if err := dependency.Check(dependency.Docker, dependency.DockerDaemon); err != nil {
		return err
	}

	_, err := runSingleStepCluster(ctx, "stop", "Stopping cluster", "stopped",
		func(ctx context.Context, cli *cluster.Client) error {
			return cli.Stop(ctx, cluster.StopOpts{})
		},
	)
	if err != nil {
		return eris.Wrap(err, "stop k3d cluster")
	}
	printer.Notificationln("(state preserved; restart with `world start`)")
	return nil
}
