package root

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	tomlpkg "github.com/argus-labs/world-engine/cli/pkg/toml"
)

type StartCmd struct {
	Debug bool `help:"Enable debug mode" default:"true" negatable:""`
}

func (c *StartCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("start-cardinal-command", map[string]any{
		"debug": c.Debug,
	})

	if err := dependency.Check(dependency.Git, dependency.Docker, dependency.DockerDaemon); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return eris.Wrap(err, "failed to get current directory")
	}
	worldCfg, err := tomlpkg.LoadFile(cwd + "/" + tomlpkg.FileName)
	if err != nil {
		return eris.Wrap(err, "load world.toml")
	}

	err = c.runK8s(ctx, cwd, worldCfg)
	if err != nil && errorspkg.ShouldPrint(err) {
		printer.Notificationln("(run `world purge` to clear cluster state)")
	}
	return err
}

////////////////////////////////////
// Pull, Build, Start Helpers //////
////////////////////////////////////

// startCluster brings up the world-agnostic platform (k3d, NATS, Traefik) in a "Cluster" box.
func startCluster(dash *phasebox.Dashboard) (*cluster.Client, error) {
	var cli *cluster.Client
	if err := dash.Run("Cluster",
		func(ctx context.Context, sess phasebox.Session) error {
			// One row per phase; k3d log lines update the current row.
			tracker := phasebox.NewStepTracker(sess)
			cli = cluster.NewClient(cluster.Config{
				LogLevel: os.Getenv("WORLD_K3D_LOG_LEVEL"),
				OnK3DLog: tracker.Detail,
			})
			err := cli.StartPlatform(ctx, tracker.Next)
			// k3d's logger is global; unhook it so late lines can't reopen the finished row.
			cli.ResetLogRouting()
			if err != nil {
				tracker.Failed(err)
				return err
			}
			tracker.Done()
			return nil
		},
		func(elapsed time.Duration) string {
			return fmt.Sprintf("ready — %s (%s)", cli.Config().ClusterName, elapsed.Round(time.Second))
		},
	); err != nil {
		return nil, eris.Wrap(err, "cluster start")
	}
	return cli, nil
}

// deployWorld applies worldCfg's operator, DB, services and ShardPools in a "World" box.
func deployWorld(dash *phasebox.Dashboard, cli *cluster.Client, worldCfg tomlpkg.Config) error {
	if err := dash.Run("World",
		func(ctx context.Context, sess phasebox.Session) error {
			tracker := phasebox.NewStepTracker(sess)
			if err := cli.DeployWorld(ctx, worldCfg, tracker.Next); err != nil {
				tracker.Failed(err)
				return err
			}
			tracker.Done()
			return nil
		},
		func(elapsed time.Duration) string {
			return fmt.Sprintf("ready — %s (%s)", worldCfg.Project, elapsed.Round(time.Second))
		},
	); err != nil {
		return eris.Wrap(err, "deploy world")
	}
	return nil
}

// deployK8sServices builds + imports + deploys every path-kind ([[services]]
// with a path=) entry from world.toml, mirroring reloadK8sShards' pull+build
// but applying Deployments via cluster.DeployServices instead of the operator
// RPC (k8s rolls the pod on image change). Image-kind services are already
// applied by cluster.Start's ensureServices. No-op when no path-kind services
// are declared. Called from runK8s right after the shard reload.
func deployK8sServices(
	dash *phasebox.Dashboard,
	cli *cluster.Client,
	dockerClient *docker.Client,
	cfg *service.Config,
) error {
	var pathKind []service.Service
	for _, svc := range cfg.WorldToml.Services {
		if svc.IsBuiltFromSource() {
			pathKind = append(pathKind, service.GameServiceFromConfig(cfg, svc))
		}
	}
	if len(pathKind) == 0 {
		return nil
	}

	return dash.Run("Services",
		func(ctx context.Context, sess phasebox.Session) error {
			// Pre-pull build dependencies (golang/runtime base images) before
			// building, same rationale as reloadK8sShards.
			if err := dockerClient.PullImages(ctx, pathKind, phasebox.PullProgress(sess)); err != nil {
				return eris.Wrap(err, "pull service build dependencies")
			}
			imageNames := docker.CardinalBuildImageNames(pathKind)
			if err := dockerClient.BuildCardinalImages(
				ctx,
				pathKind,
				phasebox.BuildProgress(sess, imageNames),
			); err != nil {
				return eris.Wrap(err, "build service images")
			}

			sess.UpsertRow("deploy", "Importing images + rolling pods", "", phasebox.Active)
			if err := cli.DeployServices(ctx, cluster.DeployServicesOpts{
				Project: cfg.WorldToml.Project,
				Config:  cfg.WorldToml,
			}); err != nil {
				sess.Fail("deploy", "Importing images + rolling pods", err)
				return eris.Wrap(err, "deploy services")
			}
			sess.UpsertRow("deploy", "Importing images + rolling pods", "", phasebox.Done)
			return nil
		},
		func(elapsed time.Duration) string {
			return fmt.Sprintf("deployed %d service(s) (%s)", len(pathKind), elapsed.Round(time.Second))
		},
	)
}

// warnConfigDBOverrides notes any POSTGRES_* env a config_db service declares:
// world-cli always overrides them with the shared-DB defaults (see cluster's
// serviceEnv), so a user's values would otherwise be silently discarded.
func warnConfigDBOverrides(cfg tomlpkg.Config) {
	for _, svc := range cfg.Services {
		if !svc.ConfigDB {
			continue
		}
		// Exactly the three keys serviceEnv force-overrides for config_db; any
		// other POSTGRES_* the user sets is passed through untouched.
		for _, key := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
			if _, ok := svc.Env[key]; ok {
				printer.Notificationf(
					"config_db service %q sets %s in [[services.env]]; world-cli overrides it with the shared-DB default.\n",
					svc.ID,
					key,
				)
			}
		}
	}
}

// warnUnforwardedServicePorts notes that [[services]] ports run in-cluster only
// — host-side port-forward is deferred (deleted with #699's PortForward rework).
// Reach them from the host with `kubectl port-forward`.
func warnUnforwardedServicePorts(cfg tomlpkg.Config) {
	for _, svc := range cfg.Services {
		for _, port := range svc.Ports {
			printer.Notificationf(
				"service %q port %d is in-cluster only — use `kubectl port-forward` to reach it from host.\n",
				svc.ID,
				port,
			)
		}
	}
}

////////////////////////////////////
// Cluster Lifecycle ///////////////
////////////////////////////////////

// runK8s brings up the k3d cluster + operator + ShardPools, reloads every
// shard from world.toml so pods leave ImagePullBackOff, then tails shard logs
// (with 'r' to reload, ctrl+r to purge + reload, without leaving the viewer).
// Blocks until Ctrl+C.
// cwd and worldCfg are resolved by Run before the "run `world purge`" hint
// becomes applicable — every error surfaced from here on is a cluster
// failure the hint can plausibly help with.
func (c *StartCmd) runK8s(ctx context.Context, cwd string, worldCfg tomlpkg.Config) error {
	warnConfigDBOverrides(worldCfg)

	// Open Docker first: shard builds don't need the cluster.
	return docker.WithClient(cwd, c.Debug, &docker.ClientOptions{Logger: logger.Slog()},
		func(cfg *service.Config, dockerClient *docker.Client) error {
			targets, err := resolveReloadTargets(cfg, nil)
			if err != nil {
				return err
			}
			services := dockerClient.ResolveServices(service.GetServices(cfg, service.CardinalShardsFirst)...)
			dockerServices := filterCardinalServicesByID(services, cfg, targets.shardIDs)

			// One dashboard spans every box this start opens (Image Pull, Build,
			// Cluster, World, Shards, Services), so adjacent boxes never visually split
			// across separate bubbletea programs. The defer covers every error
			// return below (and a panic); the success path additionally stops it
			// explicitly before warnUnforwardedServicePorts and the log picker,
			// since both write to the terminal directly and would otherwise race
			// the dashboard's still-redrawing spinner. Complete is idempotent, so
			// the deferred call after that is a no-op.
			dash := phasebox.Start(ctx, phasebox.TTY)
			defer dash.Complete()

			// Discover k3d's own cluster-bootstrap images up front so they pull
			// in the same "Image Pull" box as the shard images, instead of
			// pulling opaquely inside k3d's cluster-create. Empty (not an error)
			// when the cluster already exists — nothing new to pull on resume.
			bootstrapImages, err := cluster.NewClient(cluster.Config{
				LogLevel: os.Getenv("WORLD_K3D_LOG_LEVEL"),
			}).RequiredBootstrapImages(ctx)
			if err != nil {
				return eris.Wrap(err, "discover cluster bootstrap images")
			}

			if err := pullBuildDeps(ctx, dash, dockerClient, dockerServices, bootstrapImages); err != nil {
				return err
			}

			// Build while the platform comes up; the world deploys only after a good build.
			buildBox := dash.Open("Build")
			buildDone := make(chan error, 1)
			go func() { buildDone <- buildShardImages(buildBox, dockerClient, dockerServices) }()

			cli, clusterErr := startCluster(dash)
			buildErr := eris.Wrap(<-buildDone, "initial shard build")
			// Both halves' failures; a Ctrl+C'd half can't hide the other's.
			if err := errorspkg.JoinFailures(clusterErr, buildErr); err != nil {
				return err
			}
			if err := deployWorld(dash, cli, worldCfg); err != nil {
				return err
			}
			// NOTE: [[services]] still run in-cluster only — unlike the single shared
			// DB they're dynamic, so they can't take a fixed k3d NodePort mapping (set
			// at cluster-create). Host-side port-forward + log selection for them
			// stays deferred; reach a service port with `kubectl port-forward`.

			if err := deployShardImages(dash, cli, cfg, targets, false); err != nil {
				return eris.Wrap(err, "initial shard deploy")
			}

			// Static info, not a task — no spinner/checkmark, just the endpoint
			// URLs once the cluster + shards are up.
			resolved := cli.Config()
			endpointLines := []string{
				fmt.Sprintf("API:      %s/<organization>/<project>/<instance>", resolved.APIEndpoint),
				fmt.Sprintf("Operator: %s", resolved.OperatorEndpoint),
			}
			if service.NeedsAutoProjectDB(worldCfg) {
				// Reachable on the host via the k3d NodePort mapping — no port-forward.
				endpointLines = append(endpointLines, fmt.Sprintf("Project DB: %s", resolved.DBEndpoint))
			}
			dash.Info("Endpoints", strings.Join(endpointLines, "\n"))

			// Build + import + deploy path-kind [[services]] (e.g. services/meta);
			// no-op when none are declared.
			if err := deployK8sServices(dash, cli, dockerClient, cfg); err != nil {
				return eris.Wrap(err, "initial service deploy")
			}

			// Hand the terminal back before anything writes to it directly.
			dash.Complete()

			warnUnforwardedServicePorts(cfg.WorldToml)

			return runLogSelectionEntry(ctx, cli, cfg, dockerClient)
		},
	)
}
