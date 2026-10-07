package root

import (
	"context"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/debug"
	"github.com/argus-labs/world-engine/cli/internal/dependency"
	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/logs"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/logtail"
	tablepageselect "github.com/argus-labs/world-engine/cli/internal/tui/component/table_page_select"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

// LogsCmd shows the shard/platform log picker against an already-running world —
// the same picker `world start` drops into. ENTER tails the highlighted target,
// 'r' reloads the highlighted instance's shard, ctrl+r does the same but first
// purges just that instance's state, Ctrl+C exits (the world keeps running;
// the edge on :8080 belongs to `world start`).
type LogsCmd struct {
	Debug     bool     `help:"Enable debug mode"                                                                            default:"true" negatable:""`
	Context   string   `help:"Tail a deployed environment on this kubeconfig context instead of the local world"`
	Namespace string   `help:"Namespace on --context (defaults to the project's own, e.g. rampage-eph-<env> for ephemeral)"`
	Shard     []string `help:"Limit to these shards (remote only); repeatable"`
	Tail      int32    `help:"Historical lines to replay before tailing (remote only)"                                      default:"200"`
	Previous  bool     `help:"Logs from the previous container, for a crashed pod (remote only)"`
}

func (c *LogsCmd) Run(ctx context.Context) error {
	telemetry.PosthogCaptureEvent("logs-cardinal-command", map[string]any{
		"remote": c.Context != "",
	})

	// A deployed environment is read through kubeconfig; Docker is not involved.
	if c.Context != "" {
		worldCfg, err := loadWorldConfig()
		if err != nil {
			return err
		}
		return runRemoteLogs(ctx, c.Context, c.Namespace, worldCfg.Project, c.Shard, c.Tail, c.Previous)
	}
	if c.Namespace != "" {
		return eris.New("--namespace only applies with --context")
	}

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
			rt := local.New(dockerClient, cfg)
			running, err := rt.IsRunning(ctx)
			if err != nil {
				return eris.Wrap(err, "check world status")
			}
			if !running {
				return eris.New("world is not running — start it with `world start`")
			}
			err = runLogSelectionEntry(ctx, rt, cfg, dockerClient)
			if ctx.Err() != nil {
				return errorspkg.NewSilent(ctx.Err())
			}
			return err
		},
	)
}

// runLogSelectionEntry wires up the shard-reload callback and drops into the
// unified log picker. Shared by `world start` and `world logs`. 'r' reloads
// keeping state; ctrl+r purges first, scoped to just the highlighted instance.
func runLogSelectionEntry(
	ctx context.Context,
	rt *local.Runtime,
	cfg *service.Config,
	dockerClient *docker.Client,
) error {
	onReload := func(rctx context.Context, instanceIDs []string, purge bool) error {
		return reloadShards(rctx, rt, dockerClient, cfg, instanceIDs, purge)
	}
	return runShardLogSelectionLoop(ctx, rt, cfg, onReload)
}

// logTarget is one shard instance or platform container in the picker.
type logTarget struct {
	Instance  string // picker label, log target, and reload target
	Container string // non-shard container name (nats, db, a service); empty for shards
}

func (t logTarget) IsPlatform() bool { return t.Container != "" }

// buildLogTargets lists shard instances (one row per pool-expanded instance)
// then the platform containers (NATS, project DB, [[services]]), shards on top
// so the most-frequently-tailed items page first.
func buildLogTargets(rt *local.Runtime, cfg *service.Config) []logTarget {
	platforms := rt.PlatformContainers()
	out := make([]logTarget, 0, len(cfg.WorldToml.Shards)+len(platforms))
	for _, s := range cfg.WorldToml.Shards {
		out = append(out, logTarget{Instance: s.InstanceID})
	}
	for _, name := range platforms {
		out = append(out, logTarget{Instance: name, Container: name})
	}
	return out
}

// runShardLogSelectionLoop shows a picker with each pool instance as its own
// row plus an "All shards" aggregate. ENTER tails the selection, returning to
// the picker on exit. Reload actions target the highlighted instance.
func runShardLogSelectionLoop(
	ctx context.Context,
	rt *local.Runtime,
	cfg *service.Config,
	onReload func(ctx context.Context, instanceIDs []string, purge bool) error,
) error {
	targets := buildLogTargets(rt, cfg)

	for {
		idx, hotkey, err := selectLogTarget(ctx, targets, true)
		if err != nil {
			return errorspkg.NewSilent(err)
		}

		// Index 0 is "All shards" when includeAll=true; 1..N is each target.
		var sel *logTarget
		if idx > 0 {
			sel = &targets[idx-1]
		}

		if hotkey == hotkeyReload || hotkey == hotkeyPurgeReload {
			handleReloadHotkey(ctx, sel, onReload, hotkey == hotkeyPurgeReload)
			continue
		}

		streamer, reloadFn, extras := dispatchLogTarget(rt, cfg, sel, onReload)
		err = logs.TailLogsUntilEnterOrReload(ctx, streamer, reloadFn, extras)
		if err != nil {
			if eris.Is(err, context.Canceled) {
				return errorspkg.NewSilent(err)
			}
			return eris.Wrap(err, "tail logs")
		}
	}
}

// handleReloadHotkey processes the picker's `r` / ctrl+r hotkeys. Platform
// containers can't be reloaded from the dev loop (they are not shards) —
// surface that inline instead of silently no-op.
func handleReloadHotkey(
	ctx context.Context,
	sel *logTarget,
	onReload func(ctx context.Context, instanceIDs []string, purge bool) error,
	purge bool,
) {
	if sel != nil && sel.IsPlatform() {
		printer.Notificationf(
			"Reload not supported for %s — restart with `world stop && world start`\n",
			sel.Instance,
		)
		return
	}
	var instanceIDs []string
	label := "all shards"
	if sel != nil {
		instanceIDs = []string{sel.Instance}
		label = sel.Instance
	}
	action := "Reloading"
	if purge {
		action = "Reloading + purging"
	}
	printer.Infof("↻ %s %s...\n", action, label)
	if rerr := onReload(ctx, instanceIDs, purge); rerr != nil {
		printer.Errorf("Reload failed: %v\n", rerr)
	}
}

// dispatchLogTarget returns (streamer, reload) for the selected target.
// Platform containers stream directly and have no reload; shards reload the
// selected instance.
func dispatchLogTarget(
	rt *local.Runtime,
	cfg *service.Config,
	sel *logTarget,
	onReload func(ctx context.Context, instanceIDs []string, purge bool) error,
) (logs.StreamFn, func(context.Context, bool) error, []logtail.ExtraAction) {
	if sel != nil && sel.IsPlatform() {
		name := sel.Container
		streamer := func(ctx context.Context, out chan<- worldstatus.LogLine) error {
			return rt.StreamContainerLogs(ctx, name, worldstatus.LogsOpts{TailLines: logs.HistoryLines}, out)
		}
		return streamer, nil, nil
	}
	var instanceNames []string
	var instanceIDsForReload []string
	if sel != nil {
		instanceNames = []string{sel.Instance}
		instanceIDsForReload = []string{sel.Instance}
	}
	streamer := func(ctx context.Context, out chan<- worldstatus.LogLine) error {
		return rt.StreamShardLogs(ctx, worldstatus.LogsOpts{
			InstanceNames: instanceNames,
			TailLines:     logs.HistoryLines,
		}, out)
	}
	reloadFn := func(rctx context.Context, purge bool) error {
		return onReload(rctx, instanceIDsForReload, purge)
	}
	return streamer, reloadFn, debugActions(cfg, sel)
}

func debugActions(cfg *service.Config, sel *logTarget) []logtail.ExtraAction {
	if sel == nil {
		// There is no single instance to control when tailing all shards.
		return nil
	}
	url := local.ShardAPIURL(cfg.WorldToml.Organization, cfg.WorldToml.Project, sel.Instance)
	client := debug.NewClient(url)
	bind := func(digit, label string, action debug.Action) logtail.ExtraAction {
		return logtail.ExtraAction{
			Binding: key.NewBinding(key.WithKeys(digit), key.WithHelp(digit, label)),
			Run: func(ctx context.Context) (string, error) {
				return action(ctx, client, sel.Instance)
			},
		}
	}
	return []logtail.ExtraAction{
		bind("1", "resume", debug.Resume),
		bind("2", "pause", debug.Pause),
		bind("3", "step", debug.Step),
		bind("4", "reset", debug.Reset),
	}
}

// reloadExtraKeys binds the picker's reload hotkeys to their action tokens,
// taking the bindings from the same definition the tail view uses so 'r' means
// the same thing in both and the legend cannot drift from what is bound.
func reloadExtraKeys() []tablepageselect.ExtraKey {
	keys := logtail.DefaultReloadKeys()
	return []tablepageselect.ExtraKey{
		{Binding: keys.Reload, Action: hotkeyReload},
		{Binding: keys.PurgeReload, Action: hotkeyPurgeReload, Secondary: true},
	}
}

// Picker hotkey action tokens returned by RunWithHotkeys. 'r' rolls the
// shard keeping its state; ctrl+r wipes its JetStream state first — same
// split as the tail view's bindings.
const (
	hotkeyReload      = "reload"
	hotkeyPurgeReload = "reload-purge"
)

// selectLogTarget renders the picker over shards + platform containers.
// Each row is one target; "All shards" tops the list (platform containers are
// not included in the aggregate — they're noisy and unrelated to game logic).
func selectLogTarget(
	ctx context.Context,
	targets []logTarget,
	includeAll bool,
) (int, string, error) {
	rows := make([][]string, 0, len(targets)+1)
	if includeAll {
		rows = append(rows, []string{"All shards"})
	}
	for _, t := range targets {
		rows = append(rows, []string{t.Instance})
	}
	return tablepageselect.RunWithHotkeys(
		ctx,
		"Select instance to view logs",
		[]string{"Logs"},
		rows,
		0,
		reloadExtraKeys(),
	)
}
