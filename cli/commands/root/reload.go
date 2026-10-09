package root

import (
	"context"
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
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
)

// ReloadCmd rebuilds one or more Cardinal shards and recreates their containers
// in the running world. Inner-loop alternative to `world stop` + `world start`.
type ReloadCmd struct {
	Instances []string `arg:"" optional:"" help:"Instance IDs to reload; a pool reloads as a whole, so naming one instance restarts its siblings too. Defaults to every instance"`
	Debug     bool     `                   help:"Enable debug mode"                                                                                                               default:"true"  negatable:""`
	Purge     bool     `                   help:"Wipe instance state (NATS JetStream) before reload"                                                                              default:"false" negatable:""`
}

func (c *ReloadCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("reload-cardinal-command", map[string]any{
		"instances": c.Instances,
	})

	deps := []dependency.Dependency{dependency.Git, dependency.Docker, dependency.DockerDaemon}
	if err := dependency.Check(deps...); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return eris.Wrap(err, "failed to get current directory")
	}

	return docker.WithClient(cwd, c.Debug, &docker.ClientOptions{Logger: logger.Slog()},
		func(cfg *service.Config, dockerClient *docker.Client) error {
			// Reload covers shards only; path-kind [[services]] rebuild on `world start`.
			rt := local.New(dockerClient, cfg)
			running, err := rt.IsRunning(ctx)
			if err != nil {
				return err
			}
			if !running {
				return eris.New("world is not running — start it with `world start`")
			}
			return reloadShards(ctx, rt, dockerClient, cfg, c.Instances, c.Purge)
		},
	)
}

// reloadShards builds the targeted shards and recreates their containers. Shared
// by `world reload` and the log picker's hotkeys. instanceIDs empty means every
// instance; purge wipes the targeted instances' JetStream state first.
func reloadShards(
	ctx context.Context,
	rt *local.Runtime,
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
	dash := phasebox.Start(ctx, phasebox.TTY)
	defer dash.Complete()

	if err := pullBuildDeps(ctx, dash, dockerClient, dockerServices, nil); err != nil {
		return err
	}
	if err := buildShardImages(dash.Open("Build"), dockerClient, dockerServices); err != nil {
		return err
	}

	return deployShardImages(dash, rt, targets, purge)
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
			func(elapsed time.Duration) string {
				total := len(toPull) + len(toPullRefs)
				return fmt.Sprintf("%d image(s) pulled (%s)", total, elapsed.Round(time.Second))
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
		func(elapsed time.Duration) string {
			return fmt.Sprintf("%d image(s) built (%s)", len(dockerServices), elapsed.Round(time.Second))
		},
	)
}

// deployShardImages recreates targets' containers on their already-built images:
// an optional "Purge" box, then the "Shards" deploy.
func deployShardImages(
	dash *phasebox.Dashboard,
	rt *local.Runtime,
	targets reloadTargets,
	purge bool,
) error {
	targetIDs := targets.shardIDs
	deployShards := make([]local.DeployShard, 0, len(targetIDs))
	for _, id := range targetIDs {
		deployShards = append(deployShards, local.DeployShard{ID: id})
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
					if err := purgeAndRedeploy(ctx, sess, rt, shardID, instancesByShard[shardID]); err != nil {
						errs = append(errs, err)
					}
				}
				return errorspkg.JoinFailures(errs...)
			},
			func(elapsed time.Duration) string {
				return fmt.Sprintf("purged %d shard(s) (%s)", len(targetIDs), elapsed.Round(time.Second))
			},
		)
	}

	// Always runs, even after a purge error, so a failed wipe never leaves a shard undeployed.
	shardsErr := dash.Run("Shards",
		func(ctx context.Context, sess phasebox.Session) error {
			return rollShards(ctx, sess, rt, targets, deployShards, purge && purgeErr == nil)
		},
		func(elapsed time.Duration) string {
			return fmt.Sprintf("reloaded %d shard(s) (%s)", len(targetIDs), elapsed.Round(time.Second))
		},
	)
	// Both halves' failures; a Ctrl+C'd half can't hide the other's.
	return errorspkg.JoinFailures(eris.Wrap(purgeErr, "purge shard state"), shardsErr)
}

// purgeAndRedeploy removes the shard's containers and wipes the selected instances'
// state; the following rollShards brings it back on the new image.
func purgeAndRedeploy(
	ctx context.Context, sess phasebox.Session, rt *local.Runtime,
	shardID string, instanceIDs []string,
) error {
	sess.UpsertRow(shardID, shardID, "undeploying", phasebox.Active)
	if err := rt.UndeployShard(ctx, shardID); err != nil {
		sess.Fail(shardID, shardID, err)
		return eris.Wrapf(err, "undeploy shard %s for purge", shardID)
	}

	var errs []error
	for _, instanceID := range instanceIDs {
		sess.UpsertRow(shardID, shardID, fmt.Sprintf("wiping state for %s", instanceID), phasebox.Active)
		if err := rt.PurgeShardState(ctx, instanceID); err != nil {
			errs = append(errs, eris.Wrapf(err, "wipe state for shard %s", instanceID))
		}
	}

	// rollShards recreates the containers right after, so nothing redeploys here.
	if err := errorspkg.JoinFailures(errs...); err != nil {
		sess.Fail(shardID, shardID, err)
		return err
	}
	sess.UpsertRow(shardID, shardID, "", phasebox.Done)
	return nil
}

// rollShards recreates the targeted shards' containers, one dashboard row per shard.
func rollShards(
	ctx context.Context,
	sess phasebox.Session,
	rt *local.Runtime,
	targets reloadTargets,
	deployShards []local.DeployShard,
	waitReady bool,
) error {
	// Reload only rolls its targets, so nothing else reconciles the instance set.
	if err := rt.PruneOrphanedShards(ctx); err != nil {
		return eris.Wrap(err, "prune orphaned shards")
	}

	for _, s := range deployShards {
		sess.UpsertRow(s.ID, s.ID, "recreating containers", phasebox.Active)
	}
	if err := rt.Deploy(ctx, local.DeployOpts{
		Shards: deployShards,
		OnResult: func(shardID string, err error) {
			if err != nil {
				sess.Fail(shardID, shardID, err)
				return
			}
			sess.UpsertRow(shardID, shardID, "", phasebox.Done)
		},
	}); err != nil {
		return eris.Wrap(err, "deploy shards")
	}

	if waitReady {
		// Scoped to this reload's instances: another instance the developer left down is
		// not this reload's problem, and waiting for it would burn the whole timeout.
		ready, expected := rt.WaitForShardsReady(ctx, targets.instanceIDs, func(ready, expected int) {
			sess.UpsertRow(
				"ready",
				"Waiting for shards ready",
				fmt.Sprintf("%d/%d", ready, expected),
				phasebox.Active,
			)
		})
		if ready < expected {
			// Returned, not just shown: the summary and the exit code are what a script
			// or an agent reads, and plain progress drops the failed row entirely.
			err := eris.Errorf("%d/%d instances are serving; check `world logs`", ready, expected)
			sess.Fail("ready", "Waiting for shards ready", err)
			return err
		}
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
		want[service.CardinalShardImageName(cfg.Project, id)] = struct{}{}
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
