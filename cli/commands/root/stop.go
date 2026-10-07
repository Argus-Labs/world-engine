package root

import (
	"context"
	"os"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
)

type StopCmd struct{}

// Run stops the project's containers; volumes stay so `world start` resumes with state.
func (c *StopCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("stop-cardinal-command", map[string]any{})

	if err := dependency.Check(dependency.Docker, dependency.DockerDaemon); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return eris.Wrap(err, "failed to get current directory")
	}
	err = docker.WithClient(cwd, false, &docker.ClientOptions{Logger: logger.Slog()},
		func(cfg *service.Config, dockerClient *docker.Client) error {
			return stopWorld(ctx, local.New(dockerClient, cfg), cfg)
		},
	)
	if err != nil {
		return eris.Wrap(err, "stop world")
	}
	printer.Notificationln("(database and NATS state kept; `world start` brings the shards back)")
	return nil
}
