package root

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
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

// stopWorld stops every container of the project (volumes kept) in a one-row box.
func stopWorld(ctx context.Context, rt *local.Runtime, cfg *service.Config) error {
	return runSingleStep(ctx, cfg.WorldToml.Project, "stop", "Stopping containers", "stopped",
		func(ctx context.Context) error { return rt.Stop(ctx, nil) })
}

// runSingleStep runs one lifecycle op (`world stop`, `world purge`) through a
// one-row phasebox section; rowLabel + resultVerb becomes the collapsed summary
// (e.g. "stopped — rampage (2s)").
func runSingleStep(
	ctx context.Context,
	project, rowID, rowLabel, resultVerb string,
	op func(ctx context.Context) error,
) error {
	dash := phasebox.Start(ctx, phasebox.TTY)
	defer dash.Complete()
	return dash.Run("World",
		func(ctx context.Context, sess phasebox.Session) error {
			sess.UpsertRow(rowID, rowLabel, "", phasebox.Active)
			if opErr := op(ctx); opErr != nil {
				sess.Fail(rowID, rowLabel, opErr)
				return opErr
			}
			sess.UpsertRow(rowID, rowLabel, "", phasebox.Done)
			return nil
		},
		func(elapsed time.Duration) string {
			return fmt.Sprintf("%s — %s (%s)", resultVerb, project, elapsed.Round(time.Second))
		},
	)
}
