package root

import (
	"context"
	"os"
	"slices"

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

			return buildShards(ctx, dash, dockerClient, dockerServices)
		},
	)
}

// buildShards pulls build dependencies then builds every shard image. BuildKit
// needs the FROM images already cached, so the pull must precede the build —
// on a cold Docker cache the build otherwise fails with "no active sessions".
// Mirrors [world start] and [world reload]; the build path has no platform
// images, so extraImageRefs is nil. Only Cardinal services get pulled, since
// the full list also holds NATS and image-kind services that have no FROM
// image to pre-cache.
func buildShards(
	ctx context.Context,
	dash *phasebox.Dashboard,
	b shardBuilder,
	dockerServices []service.Service,
) error {
	buildServices := slices.DeleteFunc(slices.Clone(dockerServices), func(s service.Service) bool {
		return !docker.IsCardinalService(s)
	})
	if err := pullBuildDeps(ctx, dash, b, buildServices, nil); err != nil {
		return err
	}
	if err := buildShardImages(dash.Open("Build"), b, dockerServices); err != nil {
		return eris.Wrap(err, "Failed to build Cardinal images")
	}
	return nil
}
