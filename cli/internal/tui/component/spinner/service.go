package spinner

import (
	"context"
	"errors"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rotisserie/eris"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

// Options controls optional spinner behavior. Zero value is the legacy
// "spinner + text, no elapsed counter" mode used by the rest of the CLI.
type Options struct {
	// Elapsed enables a "(Ns)" suffix that ticks once per Bubble Tea frame.
	// Use for silent long-running ops (cluster bring-up, image import) where
	// users would otherwise wonder if the process is hung.
	Elapsed bool
}

// session implements Session for the real spinner.
type session struct {
	p    *tea.Program
	done chan struct{}
}

// Start creates, runs, and returns a spinner session. The variadic opts is
// optional — pass at most one Options value to enable elapsed-time display
// or other modes.
func Start(
	cancel context.CancelFunc,
	initialText string,
	opts ...Options,
) (Session, error) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}

	spin := Spinner{
		Spinner: spinner.New(
			spinner.WithSpinner(spinner.Dot),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("214"))),
		),
		Cancel:      cancel,
		ShowElapsed: o.Elapsed,
		Started:     time.Now(),
	}
	spin.SetText(initialText)

	p := program.NewTeaProgram(&spin)

	// Start spinner in background. Caller controls lifecycle via returned session.
	done := make(chan struct{})
	go func() {
		_, err := p.Run()
		if err != nil {
			logger.Error("failed to run spinner", "error", err)
		}
		close(done)
	}()

	return &session{p: p, done: done}, nil
}

func (s *session) Update(text string) {
	if s == nil || s.p == nil {
		return
	}
	s.p.Send(LogMsg(text))
}

func (s *session) Complete() {
	if s == nil || s.p == nil {
		return
	}
	s.p.Send(tea.Quit())
	if s.done != nil {
		<-s.done
	}
}

func (s *session) Quit() { s.Complete() }

// Run shows a spinner labeled msg while fn runs, then stops it — the convenience wrapper over
// Start/Complete for a blocking op that otherwise prints nothing (Docker build, cluster bring-up). fn
// gets a context that is cancelled if the user interrupts (Ctrl-C); on that cancellation Run returns a
// silent error so the terminal isn't flooded with a stack trace.
func Run(ctx context.Context, msg string, fn func(context.Context) error, opts ...Options) error {
	spinCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sp, err := Start(cancel, msg, opts...)
	if err != nil {
		return eris.Wrap(err, "start spinner")
	}
	opErr := fn(spinCtx)
	sp.Complete()
	// Silence Ctrl+C, not deadline expiry. Two cases are treated as "user interrupted":
	//
	//   1. spinCtx was cancelled via its cancel func — the spinner's ctx is derived with
	//      context.WithCancel, so a parent WithCancel (the real Ctrl+C path) surfaces here
	//      as context.Canceled. A parent deadline expiring surfaces as context.DeadlineExceeded,
	//      which is NOT silenced: a timeout is a real (printable) failure, not an interrupt.
	//      fn shells out via exec.CommandContext (Docker builds); once a subprocess has started,
	//      cancellation SIGKILLs it and os/exec returns *exec.ExitError ("signal: killed") whose
	//      chain never contains context.Canceled, so the cancelled spinCtx — not fn's error chain
	//      — is the source of truth for this case.
	//   2. fn cancelled its own child context and returned an error wrapping context.Canceled —
	//      the old eris.Is(opErr, context.Canceled) gate caught this, and exec.CommandContext's
	//      before-start path (os/exec returns context.Canceled when ctx is done before Start)
	//      lands here too.
	if opErr != nil && (spinCtx.Err() == context.Canceled || errors.Is(opErr, context.Canceled)) {
		return errorspkg.NewSilent(opErr)
	}
	return opErr
}
