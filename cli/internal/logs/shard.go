package logs

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/logtail"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
	"github.com/argus-labs/world-engine/cli/pkg/cluster"
)

// HistoryLines is the default history replay count on initial connect
// (capped server-side at 10000 by cardinal-operator).
const HistoryLines = 200

// StreamFn opens one log stream into out; out is closed by the streamer
// when the stream ends. Returns when the stream terminates or ctx is canceled.
type StreamFn func(ctx context.Context, out chan<- cluster.LogLine) error

// TailLogsUntilEnterOrReload runs the interactive tail view against any
// streamer (operator-mediated shard logs, direct-k8s platform logs, …).
// Returns when the user presses ENTER or ctx is canceled. On 'r' (reload) or
// ctrl+r (purge & reload), invokes onReload then re-opens the stream; reload
// errors are printed, not returned — a failed reload shouldn't end the tail
// session. onReload nil → reload hotkeys hidden + disabled. extras are the
// view's in-place actions, offered behind its legend toggle.
func TailLogsUntilEnterOrReload(
	ctx context.Context,
	openStream StreamFn,
	onReload func(ctx context.Context, purge bool) error,
	extras []logtail.ExtraAction,
) error {
	if openStream == nil {
		return eris.New("openStream cannot be nil")
	}

	for {
		tailCtx, cancel := context.WithCancel(ctx)

		linesCh := make(chan cluster.LogLine, 256)
		streamErrCh := make(chan error, 1)
		go func() {
			streamErrCh <- openStream(tailCtx, linesCh)
		}()

		m := logtail.New(ctx, onReload != nil, extras)
		prog := program.NewTeaProgram(m)
		// Forward the stream into the Program from outside it, same
		// Send-driven pattern multispinner uses for its own externally-fed
		// updates — the Program's Update loop reacts to each message.
		go pumpLogLines(prog, linesCh, streamErrCh)

		final, runErr := prog.Run()
		cancel() // stop this attempt's stream regardless of why we're here

		if ctx.Err() != nil {
			return ctx.Err()
		}
		if runErr != nil {
			return eris.Wrap(runErr, "run log tail view")
		}

		out, ok := final.(*logtail.Model)
		if !ok {
			return eris.New("failed to cast tail model")
		}

		switch {
		case out.Canceled():
			return context.Canceled
		case out.Reload():
			if rerr := onReload(ctx, out.Purge()); rerr != nil {
				printer.Errorf("Reload failed: %v\n", rerr)
			} else {
				printer.Infoln("✓ Reload complete — resuming tail")
			}
			// loop reopens the log stream against any new pods
		default:
			return nil
		}
	}
}

// pumpLogLines forwards lines from a StreamFn's output channel into prog
// until the stream closes it, then delivers the stream's terminal error.
// Safe to keep running past prog.Run() returning: Program.Send becomes a
// no-op once the Program has exited.
func pumpLogLines(prog *tea.Program, linesCh <-chan cluster.LogLine, streamErrCh <-chan error) {
	for line := range linesCh {
		prog.Send(logtail.LogLineMsg(line))
	}
	prog.Send(logtail.StreamEndedMsg{Err: <-streamErrCh})
}
