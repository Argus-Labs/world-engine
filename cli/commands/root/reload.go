package root

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/rotisserie/eris"
	"golang.org/x/sync/errgroup"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// ReloadCmd rebuilds one or more Cardinal shards and rolls them in the
// already-running local cluster. Inner-loop iteration alternative to
// `world stop` + `world start`.
type ReloadCmd struct {
	Instances []string `arg:"" optional:"" help:"Instance IDs to reload; defaults to every instance"`
	Debug     bool     `                   help:"Enable debug mode"                                  default:"true"  negatable:""`
	Purge     bool     `                   help:"Wipe instance state (NATS JetStream) before reload" default:"false" negatable:""`
}

func (c *ReloadCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("reload-cardinal-command", map[string]any{
		"instances": c.Instances,
	})

	if err := dependency.Check(dependency.Git, dependency.Docker, dependency.DockerDaemon); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return eris.Wrap(err, "failed to get current directory")
	}

	return docker.WithClient(cwd, c.Debug, &docker.ClientOptions{Logger: logger.Slog()},
		func(cfg *service.Config, dockerClient *docker.Client) error {
			// Reload covers shards only: path-kind [[services]] aren't rebuilt and
			// services dropped from world.toml aren't GC'd here — both happen on a
			// full `world start`. TODO: wire deployK8sServices + gcOrphanedServices
			// in for parity once the inner-loop UX settles.
			return reloadK8sShards(ctx, dockerClient, cfg, c.Instances, c.Purge)
		},
	)
}

// reloadK8sShards builds, imports, and rolls the targeted shards through the
// operator. Shared by `world reload` and `world start`. instanceIDs empty means
// every instance; purge wipes the targeted instances' JetStream state first.
func reloadK8sShards(
	ctx context.Context,
	dockerClient *docker.Client,
	cfg *service.Config,
	instanceIDs []string,
	purge bool,
) error {
	targets, err := resolveReloadTargets(cfg, instanceIDs)
	if err != nil {
		return err
	}
	targetIDs := targets.shardIDs

	services := dockerClient.ResolveServices(service.GetServices(cfg, service.CardinalShardsFirst)...)
	dockerServices := filterCardinalServicesByID(services, cfg, targetIDs)

	// One dashboard spans every box this reload opens (Image Pull, Build,
	// optionally Purge, Shards) through a single bubbletea program, so
	// adjacent boxes can't visually merge across a program hand-off.
	dash := phasebox.Start(ctx)
	defer dash.Complete()

	if err := pullBuildDeps(ctx, dash, dockerClient, dockerServices, nil); err != nil {
		return err
	}
	if err := buildShardImages(dash.Open("Build"), dockerClient, dockerServices); err != nil {
		return err
	}

	// OnK3DLog stays nil here: reload only calls UndeployShard/PurgeShardState/
	// DeployShard/Deploy/WaitForShardsReady — pure pkg/cluster kube calls that
	// never touch k3d's bootstrap logger (only cli.Start does, in start.go).
	cli := cluster.NewClient(cluster.Config{LogLevel: os.Getenv("WORLD_K3D_LOG_LEVEL")})
	return deployShardImages(dash, cli, cfg, targets, purge)
}

// pullBuildDeps pulls shard base images and extraImageRefs in one "Image Pull" box.
func pullBuildDeps(
	ctx context.Context,
	dash *phasebox.Dashboard,
	dockerClient *docker.Client,
	dockerServices []service.Service,
	extraImageRefs []string,
) error {
	// Dry-run both lists first: everything cached (the common case on repeat
	// runs) means there's nothing to show — skip the box entirely instead of
	// opening one that just collapses instantly to "0 image(s) pulled (0s)".
	toPull := dockerClient.ImagesToPull(ctx, dockerServices)
	toPullRefs := dockerClient.RefsToPull(ctx, extraImageRefs)

	if len(toPull) > 0 || len(toPullRefs) > 0 {
		if err := dash.Run("Image Pull",
			func(ctx context.Context, sess phasebox.Session) error {
				g, gctx := errgroup.WithContext(ctx)
				// Pre-pull build dependencies: BuildKit needs the FROM image already
				// cached, or a fresh machine's first build fails with "no active sessions".
				g.Go(func() error {
					return dockerClient.PullImages(gctx, dockerServices, phasebox.PullProgress(sess))
				})
				if len(extraImageRefs) > 0 {
					g.Go(func() error {
						return dockerClient.PullImageRefs(gctx, extraImageRefs, phasebox.PullProgress(sess))
					})
				}
				return g.Wait()
			},
			func(err error, elapsed time.Duration) (string, bool) {
				if err != nil {
					return err.Error(), true
				}
				total := len(toPull) + len(toPullRefs)
				return fmt.Sprintf("%d image(s) pulled (%s)", total, elapsed.Round(time.Second)), false
			},
		); err != nil {
			return eris.Wrap(err, "pull build dependencies")
		}
	}
	return nil
}

// buildShardImages builds every shard image in box; run pullBuildDeps first.
func buildShardImages(box *phasebox.Box, dockerClient *docker.Client, dockerServices []service.Service) error {
	return box.Run(
		func(ctx context.Context, sess phasebox.Session) error {
			imageNames := docker.CardinalBuildImageNames(dockerServices)
			return dockerClient.BuildCardinalImages(ctx, dockerServices, phasebox.BuildProgress(sess, imageNames))
		},
		func(err error, elapsed time.Duration) (string, bool) {
			if err != nil {
				return err.Error(), true
			}
			return fmt.Sprintf("%d image(s) built (%s)", len(dockerServices), elapsed.Round(time.Second)), false
		},
	)
}

// deployShardImages rolls targets' already-built images into the running
// cluster: an optional "Purge" box, then the operator "Shards" deploy.
func deployShardImages(
	dash *phasebox.Dashboard,
	cli *cluster.Client,
	cfg *service.Config,
	targets reloadTargets,
	purge bool,
) error {
	targetIDs := targets.shardIDs
	deployShards := make([]cluster.DeployShard, 0, len(targetIDs))
	for _, id := range targetIDs {
		deployShards = append(deployShards, cluster.DeployShard{
			ID:          id,
			SourceImage: service.CardinalShardImageName(cfg.Namespace, id) + ":latest",
		})
	}

	// Purge each shard independently. Redeploy still runs after purge errors so
	// a failed wipe does not leave the shard undeployed.
	var purgeErr error
	if purge {
		// Instances have separate JetStream state even when they share a shard.
		instancesByShard := make(map[string][]string, len(targetIDs))
		for _, instanceID := range targets.instanceIDs {
			shardID := targets.shardIDByInstanceID[instanceID]
			instancesByShard[shardID] = append(instancesByShard[shardID], instanceID)
		}

		purgeErr = dash.Run("Purge",
			func(ctx context.Context, sess phasebox.Session) error {
				var errs []error
				for _, shardID := range targetIDs {
					if err := purgeAndRedeploy(ctx, sess, cli, cfg, shardID, instancesByShard[shardID]); err != nil {
						errs = append(errs, err)
					}
				}
				return errors.Join(errs...)
			},
			func(err error, elapsed time.Duration) (string, bool) {
				if err != nil {
					return err.Error(), true
				}
				return fmt.Sprintf("purged %d shard(s) (%s)", len(targetIDs), elapsed.Round(time.Second)), false
			},
		)
	}

	// Always runs, even after a purge error, so the re-applied ShardPool CRs
	// get their images and leave ImagePullBackOff.
	if err := dash.Run("Shards",
		func(ctx context.Context, sess phasebox.Session) error {
			return rollShards(ctx, sess, cli, cfg, deployShards, purge && purgeErr == nil)
		},
		func(err error, elapsed time.Duration) (string, bool) {
			if err != nil {
				return err.Error(), true
			}
			return fmt.Sprintf("reloaded %d shard(s) (%s)", len(targetIDs), elapsed.Round(time.Second)), false
		},
	); err != nil {
		if errorspkg.IsSilent(err) {
			return err
		}
		if errorspkg.IsSilent(purgeErr) {
			return purgeErr
		}
		// Both halves genuinely failed — surface them together so the deploy
		// error doesn't mask an incomplete wipe.
		if purgeErr != nil {
			return errors.Join(eris.Wrap(purgeErr, "purge and redeploy shard(s)"), err)
		}
		return err
	}

	// Deploy succeeded, so the re-applied ShardPool CRs have their images and
	// the pods are rolling; only now is it safe to fail on a purge error.
	if purgeErr != nil {
		return eris.Wrap(purgeErr, "purge and redeploy shard(s)")
	}

	return nil
}

// purgeAndRedeploy wipes the selected instances, then restores the shard.
func purgeAndRedeploy(
	ctx context.Context, sess phasebox.Session, cli *cluster.Client, cfg *service.Config,
	shardID string, instanceIDs []string,
) error {
	sess.UpsertRow(shardID, shardID, "undeploying", phasebox.Active)
	if err := cli.UndeployShard(ctx, shardID); err != nil {
		sess.UpsertRow(shardID, shardID, err.Error(), phasebox.Failed)
		return eris.Wrapf(err, "undeploy shard %s for purge", shardID)
	}

	var errs []error
	for _, instanceID := range instanceIDs {
		sess.UpsertRow(shardID, shardID, fmt.Sprintf("wiping state for %s", instanceID), phasebox.Active)
		if err := cli.PurgeShardState(ctx, cfg.WorldToml.Organization, cfg.WorldToml.Project, instanceID); err != nil {
			errs = append(errs, eris.Wrapf(err, "wipe state for shard %s", instanceID))
		}
	}

	sess.UpsertRow(shardID, shardID, "redeploying", phasebox.Active)
	if err := cli.DeployShard(ctx, cfg.WorldToml, shardID); err != nil {
		errs = append(errs, eris.Wrapf(err, "redeploy shard %s", shardID))
	}

	if err := errors.Join(errs...); err != nil {
		sess.UpsertRow(shardID, shardID, err.Error(), phasebox.Failed)
		return err
	}
	sess.UpsertRow(shardID, shardID, "", phasebox.Done)
	return nil
}

// rollShards rolls the targeted shards' images via the operator, one dashboard
// row per shard.
func rollShards(
	ctx context.Context,
	sess phasebox.Session,
	cli *cluster.Client,
	cfg *service.Config,
	deployShards []cluster.DeployShard,
	waitReady bool,
) error {
	// Reload only rolls its targets, so nothing else reconciles the pool set.
	if err := cli.PruneOrphanedShards(ctx, cfg.WorldToml); err != nil {
		return eris.Wrap(err, "prune orphaned shards")
	}

	// One row per shard; OnStep's shardID changing marks the previous shard Done,
	// so whichever shard is current when Deploy errors is the one that failed.
	var current string
	if err := cli.Deploy(ctx, cluster.DeployOpts{
		Project: cfg.WorldToml.Project,
		Shards:  deployShards,
		OnStep: func(shardID, step string) {
			if current != "" && current != shardID {
				sess.UpsertRow(current, current, "", phasebox.Done)
			}
			current = shardID
			sess.UpsertRow(shardID, shardID, step, phasebox.Active)
		},
	}); err != nil {
		if current != "" {
			sess.UpsertRow(current, current, err.Error(), phasebox.Failed)
		}
		return eris.Wrap(err, "reload via operator")
	}
	if current != "" {
		sess.UpsertRow(current, current, "", phasebox.Done)
	}

	if waitReady {
		cli.WaitForShardsReady(ctx, cfg.WorldToml, func(ready, expected int) {
			sess.UpsertRow(
				"ready",
				"Waiting for pods ready",
				fmt.Sprintf("%d/%d", ready, expected),
				phasebox.Active,
			)
		})
	}
	return nil
}

type reloadTargets struct {
	shardIDs            []string
	instanceIDs         []string
	shardIDByInstanceID map[string]string
}

// resolveReloadTargets maps selected instances to their shards.
func resolveReloadTargets(cfg *service.Config, instanceIDs []string) (reloadTargets, error) {
	instances, err := cfg.WorldToml.ResolveInstanceIDs(instanceIDs)
	if err != nil {
		return reloadTargets{}, err
	}

	targets := reloadTargets{
		instanceIDs:         make([]string, 0, len(instances)),
		shardIDByInstanceID: make(map[string]string, len(instances)),
	}
	seenShard := make(map[string]bool, len(instances))
	for _, instance := range instances {
		if !seenShard[instance.ID] {
			seenShard[instance.ID] = true
			targets.shardIDs = append(targets.shardIDs, instance.ID)
		}
		targets.instanceIDs = append(targets.instanceIDs, instance.InstanceID)
		targets.shardIDByInstanceID[instance.InstanceID] = instance.ID
	}
	return targets, nil
}

// filterCardinalServicesByID returns the subset of services whose image name
// matches one of the target shard IDs.
func filterCardinalServicesByID(services []service.Service, cfg *service.Config, targetIDs []string) []service.Service {
	want := make(map[string]struct{}, len(targetIDs))
	for _, id := range targetIDs {
		want[service.CardinalShardImageName(cfg.Namespace, id)] = struct{}{}
	}
	filtered := make([]service.Service, 0, len(targetIDs))
	for _, s := range services {
		if _, ok := want[s.Image]; ok &&
			slices.IndexFunc(filtered, func(x service.Service) bool { return x.Image == s.Image }) < 0 {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
