package phasebox

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rotisserie/eris"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

// Dashboard is one continuous bubbletea program spanning every section a
// command opens (e.g. "Image Pull", "Build", "Cluster", "Shards" for
// `world start`), rendered through a single Model so there's no hand-off
// gap where one box's border could visually merge with the next.
type Dashboard struct {
	p      *tea.Program
	done   chan struct{}
	ctx    context.Context //nolint:containedctx // shared across every Run call in this dashboard's lifetime, mirroring spinner/multispinner's cancel-scoped session pattern
	nextID atomic.Int64
	once   sync.Once
}

// Box is an opened section; opening it before Run fixes its place in the order.
type Box struct {
	d  *Dashboard
	id string
}

// Start opens a dashboard scoped to ctx: Ctrl+C cancels the shared
// context, so a cancellation is visible to every later section too, not
// just the active one. Callers should `defer dash.Complete()` immediately.
func Start(ctx context.Context) *Dashboard {
	dctx, cancel := context.WithCancel(ctx)
	p := program.NewTeaProgram(newModel(cancel), tea.WithContext(dctx))
	done := make(chan struct{})
	go func() {
		if _, err := p.Run(); err != nil {
			logger.Error("failed to run phasebox dashboard", "error", err)
		}
		close(done)
	}()
	return &Dashboard{p: p, done: done, ctx: dctx}
}

// Complete stops the dashboard's program, leaving every section's final
// frame — rows and all — in the terminal scrollback. Idempotent: callers
// `defer dash.Complete()` and may also call it early to release the
// terminal before writing to it directly, so the deferred second call must
// be a no-op.
func (d *Dashboard) Complete() {
	d.once.Do(func() {
		d.p.Send(tea.Quit())
		<-d.done
	})
}

// Open appends a titled section.
func (d *Dashboard) Open(title string) *Box {
	id := strconv.FormatInt(d.nextID.Add(1), 10)
	d.p.Send(newSectionMsg{id: id, title: title})
	return &Box{d: d, id: id}
}

// Run opens and runs a section; see Box.Run.
func (d *Dashboard) Run(
	title string,
	fn func(ctx context.Context, sess Session) error,
	summarize func(err error, elapsed time.Duration) (summary string, failed bool),
) error {
	return d.Open(title).Run(fn, summarize)
}

// Run runs fn with a Session scoped to this box, then appends summarize's
// result below its rows (which stay visible, not replaced). Mirrors
// spinner.Run's shape: fn gets the dashboard's shared, Ctrl+C-cancelable
// context, and a resulting context.Canceled becomes a silent error instead
// of a printed stack trace.
func (b *Box) Run(
	fn func(ctx context.Context, sess Session) error,
	summarize func(err error, elapsed time.Duration) (summary string, failed bool),
) error {
	sess := &sectionSession{p: b.d.p, section: b.id}

	started := time.Now()
	opErr := fn(b.d.ctx, sess)
	summary, failed := summarize(opErr, time.Since(started))
	b.d.p.Send(collapseMsg{section: b.id, summary: summary, failed: failed})

	if eris.Is(opErr, context.Canceled) {
		return errorspkg.NewSilent(opErr)
	}
	return opErr
}

// Info adds a titled section showing body as static, finished content —
// no spinner, rows, or icon. For content that isn't a pass/fail task (e.g.
// endpoint URLs), since Run's summary always carries a ✓/✗ icon that reads
// oddly there.
func (d *Dashboard) Info(title, body string) {
	b := d.Open(title)
	d.p.Send(infoMsg{section: b.id, body: body})
}

// sectionSession is the Session implementation for one section within a
// Dashboard — every UpsertRow/UpsertProgress call is tagged with its
// section id so Update routes it to the right box.
type sectionSession struct {
	p       *tea.Program
	section string
}

func (s *sectionSession) UpsertRow(id, label, detail string, state RowState) {
	s.p.Send(rowMsg{section: s.section, id: id, label: label, detail: detail, state: state})
}

func (s *sectionSession) UpsertProgress(id, label string, percent int) {
	s.p.Send(progressMsg{section: s.section, id: id, label: label, percent: percent})
}
