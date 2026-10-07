package root

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// BuildCmd builds Cardinal shard images without starting any containers.
// Hidden from `world -h` because it exists for deployment CI, not interactive
// use — `world start` is the supported user flow.
type BuildCmd struct {
	Debug    bool              `default:"true" help:"Enable debug mode"                                                                          negatable:""`
	Shard    string            `               help:"Build only the specified shard ID (defaults to all shards in world.toml)"`
	Progress phasebox.Progress `default:"tty"  help:"How to show progress: tty (live box) or plain (one line per finished section, for CI logs)"              enum:"tty,plain"`
}

func (c *BuildCmd) Run(ctx context.Context) error {
	if err := dependency.Check(
		dependency.Go,
		dependency.Git,
		dependency.Docker,
		dependency.DockerDaemon,
	); err != nil {
		return err
	}

	telemetry.PosthogCaptureEvent("build-cardinal-command", map[string]any{
		"debug": c.Debug,
		"shard": c.Shard,
	})

	cwd, err := os.Getwd()
	if err != nil {
		return eris.Wrap(err, "failed to get current directory")
	}

	return docker.WithClient(cwd, c.Debug, &docker.ClientOptions{Logger: logger.Slog()},
		func(cfg *service.Config, dockerClient *docker.Client) error {
			services := service.GetServices(cfg, service.CardinalShardsFirst)
			dockerServices := dockerClient.ResolveServices(services...)

			if c.Shard != "" {
				if !slices.ContainsFunc(cfg.WorldToml.Shards, func(s worldtoml.Shard) bool {
					return s.ID == c.Shard
				}) {
					return eris.Errorf("shard %q not found in world.toml", c.Shard)
				}

				wantImage := service.CardinalShardImageName(cfg.Project, c.Shard)
				filtered := make([]service.Service, 0, 1)
				for _, s := range dockerServices {
					if s.Image == wantImage {
						filtered = append(filtered, s)
					}
				}
				dockerServices = filtered
			}

			if !slices.ContainsFunc(dockerServices, docker.IsCardinalService) {
				return eris.New("no Cardinal shards found in this project")
			}

			dash := phasebox.Start(ctx, c.Progress)
			defer dash.Complete()

			if err := dash.Run("Build",
				func(ctx context.Context, sess phasebox.Session) error {
					imageNames := docker.CardinalBuildImageNames(dockerServices)
					return dockerClient.BuildCardinalImages(
						ctx,
						dockerServices,
						phasebox.BuildProgress(sess, imageNames),
					)
				},
				func(elapsed time.Duration) string {
					return fmt.Sprintf("%d image(s) built (%s)", len(dockerServices), elapsed.Round(time.Second))
				},
			); err != nil {
				return eris.Wrap(err, "Failed to build Cardinal images")
			}
			return nil
		},
	)
}
