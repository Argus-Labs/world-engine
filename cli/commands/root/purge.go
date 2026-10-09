package root

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/moby/moby/client"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

type PurgeCmd struct {
	Image bool `help:"Also prune Cardinal shard images + per-reload registry tags. k3d base images are NOT removed."`
}

func (c *PurgeCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("purge-cardinal-command", map[string]any{
		"image": c.Image,
	})

	if err := dependency.Check(dependency.Docker, dependency.DockerDaemon); err != nil {
		return err
	}

	cli, err := runSingleStepCluster(ctx, "purge", "Deleting cluster", "deleted",
		func(ctx context.Context, cli *cluster.Client) error {
			return cli.Purge(ctx, cluster.PurgeOpts{})
		},
	)
	if err != nil {
		return eris.Wrap(err, "purge k3d cluster")
	}
	printer.Notificationln("(all state wiped)")

	if !c.Image {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return eris.Wrap(err, "failed to get current directory")
	}
	return docker.WithClient(cwd, false, &docker.ClientOptions{Logger: logger.Slog()},
		func(cfg *service.Config, dockerClient *docker.Client) error {
			return purgeK8sImages(ctx, dockerClient, cfg, cli.Config().RegistryName)
		},
	)
}

// purgeK8sImages cleans up the local docker images created by `world reload`.
// Two passes:
//
//  1. Cardinal-labeled shard source images (e.g. rampage-backend-gameplay-shard:latest)
//     via the existing dockerClient.PruneCardinalImages — same logic the docker
//     backend's --image flag uses.
//  2. The retagged k3d-registry push tags this stack creates per reload for each
//     shard and path-kind service
//     (e.g. k3d-world-engine-registry.localhost:5000/rampage/gameplay:local-1780199123).
//     One tag accumulates per reload + per ID, so over a long dev session
//     this can be 100+ images each pointing at a different ID. Listed via
//     reference-filter and best-effort-removed.
//
// k3d base images (rancher/k3s, k3d-tools, k3d-proxy) are intentionally NOT
// removed — they're expensive to re-pull (~600 MB on cold start).
func purgeK8sImages(ctx context.Context, dockerClient *docker.Client, cfg *service.Config, registryName string) error {
	if err := dockerClient.PruneCardinalImages(ctx); err != nil {
		return eris.Wrap(err, "prune Cardinal images")
	}
	printer.Step("pruned", "Cardinal", "images")

	ids := cfg.WorldToml.ShardIDs()
	for _, svc := range cfg.WorldToml.Services {
		if svc.IsBuiltFromSource() {
			ids = append(ids, svc.ID)
		}
	}
	removed, err := removeRegistryPushTags(ctx, registryName, cfg.WorldToml.Project, ids)
	if err != nil {
		return eris.Wrap(err, "remove registry push tags")
	}
	if removed > 0 {
		printer.Step("removed", "registry tag(s)", strconv.Itoa(removed))
	}
	return nil
}

// removeRegistryPushTags removes every host-side docker image tag matching
// the per-ID push prefix the reload path uses (`k3d-<registry>.localhost
// :5000/<project>/<id>:*`). Best-effort: a single tag failing doesn't
// abort the rest. Returns the number actually removed.
func removeRegistryPushTags(ctx context.Context, registryName, project string, ids []string) (int, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return 0, eris.Wrap(err, "docker client")
	}
	defer func() { _ = cli.Close() }()

	count := 0
	for _, id := range ids {
		ref := fmt.Sprintf("k3d-%s.localhost:5000/%s/%s", registryName, project, id)
		list, lerr := cli.ImageList(ctx, client.ImageListOptions{
			Filters: make(client.Filters).Add("reference", ref+":*"),
		})
		if lerr != nil {
			continue
		}
		for _, img := range list.Items {
			if _, rerr := cli.ImageRemove(
				ctx,
				img.ID,
				client.ImageRemoveOptions{PruneChildren: true, Force: true},
			); rerr == nil {
				count++
			}
		}
	}
	return count, nil
}
