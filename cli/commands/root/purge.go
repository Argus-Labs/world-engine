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

type PurgeCmd struct {
	Image bool `help:"Also remove the images world built for this project's shards and services."`
}

// Run removes the project's containers, volumes and network.
func (c *PurgeCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("purge-cardinal-command", map[string]any{
		"image": c.Image,
	})

	if err := dependency.Check(dependency.Docker, dependency.DockerDaemon); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return eris.Wrap(err, "failed to get current directory")
	}
	err = docker.WithClient(cwd, false, &docker.ClientOptions{Logger: logger.Slog()},
		func(cfg *service.Config, dockerClient *docker.Client) error {
			rt := local.New(dockerClient, cfg)
			return runSingleStep(
				ctx,
				cfg.WorldToml.Project,
				"purge",
				"Removing containers, volumes and network",
				"purged",
				func(ctx context.Context) error { return rt.Purge(ctx, c.Image, nil) },
			)
		},
	)
	if err != nil {
		return eris.Wrap(err, "purge world")
	}
	printer.Notificationln("(all state wiped)")
	return nil
}
