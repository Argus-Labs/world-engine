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
	"github.com/argus-labs/world-engine/cli/internal/tui/style"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
	tomlpkg "github.com/argus-labs/world-engine/cli/pkg/toml"
)

type StartCmd struct {
	Debug bool `help:"Enable debug mode" default:"true" negatable:""`
}

// Run brings the world up on Docker and stays attached: the edge proxy lives in
// this process, so Ctrl+C (or quitting the log picker) stops the containers again,
// keeping their volumes, like `docker compose up`.
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
	worldCfg, err := tomlpkg.LoadDir(cwd)
	if err != nil {
		return eris.Wrap(err, "load world.toml")
	}
	warnConfigDBOverrides(worldCfg)

	err = c.run(ctx, cwd)
	if err != nil && errorspkg.ShouldPrint(err) {
		printer.Notificationln("(run `world purge` to clear local state)")
	}
	return err
}

func (c *StartCmd) run(ctx context.Context, cwd string) error {
	return docker.WithClient(cwd, c.Debug, &docker.ClientOptions{Logger: logger.Slog()},
		func(cfg *service.Config, dockerClient *docker.Client) error {
			rt := local.New(dockerClient, cfg)
			targets, err := resolveReloadTargets(cfg, nil)
			if err != nil {
				return err
			}
			services := dockerClient.ResolveServices(service.GetServices(cfg, service.CardinalShardsFirst)...)
			dockerServices := filterCardinalServicesByID(services, cfg, targets.shardIDs)

			// One dashboard spans every box (Image Pull, Build, Platform, Shards,
			// Services); Complete hands the terminal back before the log picker.
			dash := phasebox.Start(ctx, phasebox.TTY)
			defer dash.Complete()

			if err := pullBuildDeps(ctx, dash, dockerClient, dockerServices, platformImageRefs(cfg)); err != nil {
				return err
			}

			// Build while NATS and Postgres come up; shards deploy only after a good build.
			buildBox := dash.Open("Build")
			buildDone := make(chan error, 1)
			go func() { buildDone <- buildShardImages(buildBox, dockerClient, dockerServices) }()

			platformErr := startPlatform(dash, rt, cfg)
			buildErr := <-buildDone
			if platformErr != nil {
				return platformErr
			}
			if buildErr != nil {
				return eris.Wrap(buildErr, "initial shard build")
			}

			if err := deployShardImages(dash, rt, targets, false); err != nil {
				return eris.Wrap(err, "initial shard deploy")
			}
			if err := deployServices(dash, rt, dockerClient, cfg); err != nil {
				return eris.Wrap(err, "initial service deploy")
			}

			// The edge is the only entry point; it dies with this process.
			edgeCtx, stopEdge := context.WithCancel(ctx)
			defer stopEdge()
			edgeErr := make(chan error, 1)
			go func() { edgeErr <- rt.ServeEdge(edgeCtx) }()
			select {
			case err := <-edgeErr:
				return eris.Wrap(err, "start edge")
			case <-time.After(200 * time.Millisecond):
			}

			dash.Complete()
			// Its own box, sized to its lines: the dashboard's width is frozen by now.
			printer.Infoln(style.MultiSectionBox([]style.Section{
				{Title: "Endpoints", Body: strings.Join(endpointLines(cfg), "\n")},
			}))

			err = runLogSelectionEntry(ctx, rt, cfg, dockerClient)
			if ctx.Err() != nil {
				err = errorspkg.NewSilent(ctx.Err()) // Ctrl+C is the normal way out
			}
			// The edge dying mid-session makes every client call fail with connection
			// refused, which looks like a shard problem unless we say otherwise.
			select {
			case edgeFailure := <-edgeErr:
				if edgeFailure != nil {
					printer.Errorf("Edge proxy stopped: %v\n", edgeFailure)
				}
			default:
			}

			// Attached semantics: leaving world start stops the world, volumes stay.
			stopCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			stopEdge()
			if stopErr := stopWorld(stopCtx, rt, cfg); stopErr != nil {
				printer.Errorf("Stop failed: %v\n", stopErr)
			}
			return err
		},
	)
}

// startPlatform starts the network, NATS, Postgres and image-kind services in a "Platform" box.
func startPlatform(dash *phasebox.Dashboard, rt *local.Runtime, cfg *service.Config) error {
	if err := dash.Run("Platform",
		func(ctx context.Context, sess phasebox.Session) error {
			tracker := phasebox.NewStepTracker(sess)
			if err := rt.StartPlatform(ctx, tracker.Next); err != nil {
				tracker.Failed(err)
				return err
			}
			tracker.Done()
			return nil
		},
		func(elapsed time.Duration) string {
			return fmt.Sprintf("ready — %s (%s)", cfg.WorldToml.Project, elapsed.Round(time.Second))
		},
	); err != nil {
		return eris.Wrap(err, "start platform")
	}
	return nil
}

// platformImageRefs lists the pulled images the platform needs, so Image Pull shows them.
func platformImageRefs(cfg *service.Config) []string {
	refs := []string{service.NATS(cfg).Image}
	if service.NeedsAutoProjectDB(cfg.WorldToml) {
		refs = append(refs, service.ProjectDBService(cfg).Image)
	}
	for _, gs := range cfg.WorldToml.Services {
		if !gs.IsBuiltFromSource() {
			refs = append(refs, gs.Image)
		}
	}
	return refs
}

// deployServices builds and (re)creates path-kind [[services]] in a "Services" box; no-op without any.
func deployServices(
	dash *phasebox.Dashboard,
	rt *local.Runtime,
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
			sess.UpsertRow("deploy", "Starting containers", "", phasebox.Active)
			if err := rt.DeployServices(ctx); err != nil {
				return eris.Wrap(err, "deploy services")
			}
			sess.UpsertRow("deploy", "Starting containers", "", phasebox.Done)
			return nil
		},
		func(elapsed time.Duration) string {
			return fmt.Sprintf("deployed %d service(s) (%s)", len(pathKind), elapsed.Round(time.Second))
		},
	)
}

// endpointLines is the static "where things are" box printed after the dashboard.
func endpointLines(cfg *service.Config) []string {
	org, project := local.Sanitized(cfg.WorldToml.Organization, cfg.WorldToml.Project)
	lines := []string{
		fmt.Sprintf("API:  %s/%s/%s/<instance>", local.APIEndpoint, org, project),
		fmt.Sprintf("NATS: %s", local.NatsHostURL),
	}
	if service.NeedsAutoProjectDB(cfg.WorldToml) {
		lines = append(
			lines,
			fmt.Sprintf("DB:   postgres://postgres:postgres@%s/%s", local.DBEndpoint, cfg.WorldToml.Project),
		)
	}
	for i, sh := range cfg.WorldToml.Shards {
		lines = append(lines, fmt.Sprintf("%-5s 127.0.0.1:%d", sh.InstanceID+":", service.ShardHostPort(i)))
	}
	for _, gs := range cfg.WorldToml.Services {
		for _, port := range gs.Ports {
			lines = append(lines, fmt.Sprintf("%-5s localhost:%d", gs.ID+":", port))
		}
	}
	return lines
}

// warnConfigDBOverrides notes any POSTGRES_* env a config_db service declares:
// world start always overrides them with the shared-DB defaults, so a user's
// values would otherwise be silently discarded.
func warnConfigDBOverrides(cfg tomlpkg.Config) {
	for _, svc := range cfg.Services {
		if !svc.ConfigDB {
			continue
		}
		for _, key := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
			if _, ok := svc.Env[key]; ok {
				printer.Notificationf(
					"config_db service %q sets %s in [[services.env]]; world start overrides it with the shared-DB default.\n",
					svc.ID,
					key,
				)
			}
		}
	}
}
