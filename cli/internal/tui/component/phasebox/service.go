package phasebox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

// Progress is how a Dashboard shows progress.
type Progress string

const (
	// TTY is the live box, redrawn in place; it needs an interactive terminal.
	TTY Progress = "tty"
	// Plain prints one line per finished section, for logs that can't redraw,
	// such as CI's.
	Plain Progress = "plain"
)

// Dashboard is one continuous bubbletea program spanning every section a
// command opens (e.g. "Build", "Platform", "Services" for
// `world start`), rendered through a single Model so there's no hand-off
// gap where one box's border could visually merge with the next. With Plain
// progress there's no program; each section prints one summary line.
type Dashboard struct {
	p      *tea.Program // nil with Plain progress
	plain  io.Writer    // Plain progress's output; nil for the live box
	done   chan struct{}
	ctx    context.Context //nolint:containedctx // shared across every Run call in this dashboard's lifetime, mirroring spinner/multispinner's cancel-scoped session pattern
	nextID atomic.Int64
	once   sync.Once
}

// Box is an opened section; opening it before Run fixes its place in the order.
type Box struct {
	d     *Dashboard
	id    string
	title string
}

// Start opens a dashboard scoped to ctx, shown per progress. Ctrl+C cancels
// the shared context, so a cancellation is visible to every later section
// too, not just the active one. Callers should `defer dash.Complete()`
// immediately.
func Start(ctx context.Context, progress Progress) *Dashboard {
	done := make(chan struct{})
	if progress == Plain {
		// No keyboard to read; a Ctrl+C arrives as a signal, which cancels ctx.
		close(done)
		return &Dashboard{plain: os.Stdout, done: done, ctx: ctx}
	}

	dctx, cancel := context.WithCancel(ctx)
	p := program.NewTeaProgram(newModel(cancel), tea.WithContext(dctx))
	go func() {
		if _, err := p.Run(); err != nil {
			logger.Error("failed to run phasebox dashboard", "error", err)
		}
		close(done)
	}()
	return &Dashboard{p: p, done: done, ctx: dctx}
}

// send passes msg to the live box; Plain progress has none.
func (d *Dashboard) send(msg tea.Msg) {
	if d.p != nil {
		d.p.Send(msg)
	}
}

// Complete stops the dashboard's program, leaving every section's final
// frame — rows and all — in the terminal scrollback. Idempotent: callers
// `defer dash.Complete()` and may also call it early to release the
// terminal before writing to it directly, so the deferred second call must
// be a no-op.
func (d *Dashboard) Complete() {
	d.once.Do(func() {
		d.send(tea.Quit())
		<-d.done
	})
}

// Open appends a titled section.
func (d *Dashboard) Open(title string) *Box {
	id := strconv.FormatInt(d.nextID.Add(1), 10)
	d.send(newSectionMsg{id: id, title: title})
	return &Box{d: d, id: id, title: title}
}

// Run opens and runs a section; see Box.Run.
func (d *Dashboard) Run(
	title string,
	fn func(ctx context.Context, sess Session) error,
	summarize func(elapsed time.Duration) string,
) error {
	return d.Open(title).Run(fn, summarize)
}

// Run runs fn with a Session scoped to this box, then appends a summary below
// its rows (which stay visible, not replaced): summarize's on success, a fixed
// failed/canceled line otherwise, since the error is printed after the
// dashboard. fn gets the dashboard's shared, Ctrl+C-cancelable context, and a
// resulting [context.Canceled] becomes a silent error.
func (b *Box) Run(
	fn func(ctx context.Context, sess Session) error,
	summarize func(elapsed time.Duration) string,
) error {
	sess := &sectionSession{d: b.d, section: b.id}

	started := time.Now()
	opErr := fn(b.d.ctx, sess)
	elapsed := time.Since(started)

	var summary string
	switch {
	case opErr == nil:
		summary = summarize(elapsed)
	case isCanceled(opErr):
		summary = "canceled"
	default:
		summary = fmt.Sprintf("failed (%s) — see error below", elapsed.Round(time.Second))
	}
	b.d.send(collapseMsg{section: b.id, summary: summary, failed: opErr != nil})
	if b.d.plain != nil {
		icon := style.TickIcon.Render()
		if opErr != nil {
			icon = style.CrossIcon.Render()
		}
		fmt.Fprintf(b.d.plain, "%s: %s%s\n", b.title, icon, summary)
	}

	if isCanceled(opErr) {
		return errorspkg.NewSilent(opErr)
	}
	return opErr
}

// sectionSession is the Session implementation for one section within a
// Dashboard — every UpsertRow/UpsertProgress call is tagged with its
// section id so Update routes it to the right box. Plain progress drops rows.
type sectionSession struct {
	d       *Dashboard
	section string
}

func (s *sectionSession) UpsertRow(id, label, detail string, state RowState) {
	s.d.send(rowMsg{section: s.section, id: id, label: label, detail: detail, state: state})
}

func (s *sectionSession) UpsertProgress(id, label string, percent int) {
	s.d.send(progressMsg{section: s.section, id: id, label: label, percent: percent})
}

func (s *sectionSession) Fail(id, label string, err error) {
	detail := "failed"
	if isCanceled(err) {
		detail = "canceled"
	}
	s.d.send(rowMsg{section: s.section, id: id, label: label, detail: detail, state: failed})
}

// isCanceled uses errors.Is, not eris.Is, so it also sees into errors.Join.
func isCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}
