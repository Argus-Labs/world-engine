package phasebox

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

// Row is one line within a section, keyed externally by an id (see rowMsg /
// progressMsg). Progress is non-nil only for a percent-bar row (set by
// UpsertProgress); a later plain UpsertRow call for the same id replaces the
// Row wholesale, implicitly clearing it back to an icon-rendered row.
type Row struct {
	Label    string
	Detail   string
	State    RowState
	Progress *int
}

// section is one titled region of the dashboard's single box (see
// style.MultiSectionBox): the first section's title sits on the top
// border, later ones on divider lines. Dashboard.Run opens one per call;
// once finished, a summary line is appended below its rows, which stay
// visible so the box keeps showing what happened.
//
// Dashboard.Info opens a plain section instead — no rows, no ✓/✗ icon —
// for static content (e.g. endpoint URLs) that isn't a pass/fail task.
type section struct {
	id       string
	title    string
	rowOrder []string
	rows     map[string]Row

	finished bool
	summary  string
	failed   bool
	plain    bool
}

// Model is the bubbletea model backing a Dashboard — one continuous
// program spanning every section opened during a command's run, rendered
// as a single box (style.MultiSectionBox). Routing every section through
// one Model instead of one tea.Program per section avoids the hand-off gap
// where a second program's first frame could land on the first program's
// last row.
type Model struct {
	sections    []section
	sectionByID map[string]int

	// width/height come from tea.WindowSizeMsg (0 until the first one
	// arrives). innerWidth is 0 while the box is still free to shrink to fit
	// its content, and is frozen the moment a section is committed to
	// scrollback — those lines can't be re-rendered, so the live piece must
	// keep matching them. continued records that something was printed above,
	// so the live piece opens with a divider instead of a top corner.
	width      int
	height     int
	innerWidth int
	continued  bool

	spinner spinner.Model
	cancel  func()
}

const (
	// maxBoxWidth keeps the box readable on a very wide terminal — past this,
	// full-width rows are harder to scan than they are useful.
	maxBoxWidth = 120
	// heightMargin leaves the shell a couple of lines below the box (View's
	// trailing spacer, then the prompt) so the frame isn't flush to the edge.
	heightMargin = 2
)

// newSectionMsg opens section id (title) as the newest section, appended
// after any earlier ones.
type newSectionMsg struct {
	id, title string
}

// rowMsg upserts one row within section: first occurrence of id appends it
// (in arrival order within that section); later occurrences update
// label/detail/state in place.
type rowMsg struct {
	section, id, label, detail string
	state                      RowState
}

// progressMsg upserts one row within section as a percent-bar (see
// Session.UpsertProgress).
type progressMsg struct {
	section, id, label string
	percent            int
}

// collapseMsg marks section finished and appends its summary line (rows are
// left in place — see section's doc comment).
type collapseMsg struct {
	section, summary string
	failed           bool
}

// infoMsg marks section finished with plain static content — no icon, no
// rows (see Dashboard.Info).
type infoMsg struct {
	section, body string
}

func newModel(cancel func()) Model {
	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(style.Yellow))),
	)
	return Model{
		sectionByID: make(map[string]int),
		spinner:     s,
		cancel:      cancel,
	}
}

func (m Model) Init() tea.Cmd { return m.spinner.Tick }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case newSectionMsg:
		m.sectionByID[msg.id] = len(m.sections)
		m.sections = append(m.sections, section{id: msg.id, title: msg.title, rows: make(map[string]Row)})
		return m.flushToFit()
	case rowMsg:
		// Only a new row can grow the frame, so only that needs a flush check —
		// see upsert's contract.
		if m.upsert(msg.section, msg.id, Row{Label: msg.label, Detail: msg.detail, State: msg.state}) {
			return m.flushToFit()
		}
	case progressMsg:
		percent := msg.percent
		if m.upsert(msg.section, msg.id, Row{Label: msg.label, State: Active, Progress: &percent}) {
			return m.flushToFit()
		}
	case collapseMsg:
		return m.finish(msg.section, msg.summary, msg.failed, false).flushToFit()
	case infoMsg:
		return m.finish(msg.section, msg.body, false, true).flushToFit()
	}
	return m, nil
}

// finish marks a section done and appends its summary line below its rows,
// which stay visible. Unknown sections — already finished and committed to
// scrollback — are ignored.
func (m Model) finish(sectionID, summary string, failed, plain bool) Model {
	i, ok := m.sectionByID[sectionID]
	if !ok {
		return m
	}
	sec := &m.sections[i]
	sec.finished = true
	sec.summary = summary
	sec.failed = failed
	sec.plain = plain
	return m
}

// upsert appends row under id in section, or updates it in place if that id
// is already there, reporting whether it appended. Only an append changes
// the frame's height — style.contentLine truncates rather than wraps, so a
// row is always exactly one line no matter how its label/detail changes —
// which is what lets Update skip flushToFit on the far more frequent
// in-place updates (progress ticks, k3d log lines).
func (m Model) upsert(sectionID, id string, row Row) bool {
	i, ok := m.sectionByID[sectionID]
	if !ok || m.sections[i].finished {
		return false
	}
	sec := &m.sections[i]
	_, exists := sec.rows[id]
	if !exists {
		sec.rowOrder = append(sec.rowOrder, id)
	}
	sec.rows[id] = row
	return !exists
}

func (m Model) View() string {
	out, _ := style.MultiSectionBoxOpts(m.styleSections(m.sections), m.boxOpts(false))
	// Trailing newline: bubbletea's quit teardown erases the cursor's line,
	// which without a spacer would wipe the box's bottom border on the last
	// frame.
	return out + "\n"
}

// boxOpts clamps the box to the terminal but does NOT dictate its width:
// innerWidth stays 0 until a section is committed to scrollback, so until
// then the box shrinks to fit its content the way a single-frame box does.
func (m Model) boxOpts(open bool) style.BoxOpts {
	opts := style.BoxOpts{InnerWidth: m.innerWidth, Continued: m.continued, Open: open}
	if m.width > 0 {
		opts.MaxWidth = min(m.width, maxBoxWidth)
	}
	return opts
}

// styleSections adapts sections for rendering.
func (m Model) styleSections(secs []section) []style.Section {
	out := make([]style.Section, len(secs))
	for i, sec := range secs {
		out[i] = style.Section{Title: sec.title, Body: renderSection(sec, m.spinner)}
	}
	return out
}

// sectionHeight is the section's divider/title line plus its body lines.
func (m Model) sectionHeight(sec section) int {
	return 1 + strings.Count(renderSection(sec, m.spinner), "\n") + 1
}

// liveHeight is how tall the current frame renders, bottom border included.
func (m Model) liveHeight() int {
	h := 1
	for _, sec := range m.sections {
		h += m.sectionHeight(sec)
	}
	return h
}

// flushToFit commits leading finished sections to scrollback, but only once
// the live frame would outgrow the terminal. bubbletea's renderer drops lines
// off the TOP of an over-tall frame, and those lines only ever existed inside
// a live frame — so without this the earliest sections are both clipped from
// the screen and absent from scrollback afterwards.
//
// Staying live while it fits is what keeps the box shrink-to-fit: a section
// still in the frame can be re-rendered wider by a later one, a section
// already in scrollback cannot. Most runs are short enough to never flush.
func (m Model) flushToFit() (Model, tea.Cmd) {
	if m.height <= 0 || m.liveHeight() <= m.height-heightMargin {
		return m, nil
	}

	// Never commit the in-flight section: it can still gain rows.
	maxFlush := 0
	for maxFlush < len(m.sections)-1 && m.sections[maxFlush].finished {
		maxFlush++
	}
	n, h := 0, m.liveHeight()
	for n < maxFlush && h > m.height-heightMargin {
		h -= m.sectionHeight(m.sections[n])
		n++
	}
	if n == 0 {
		return m, nil
	}

	// Size against every section still in play, not just the ones going out,
	// so the piece left live lines up with what lands above it. From here the
	// width is frozen — scrollback can't be re-rendered.
	_, inner := style.MultiSectionBoxOpts(m.styleSections(m.sections), m.boxOpts(false))
	m.innerWidth = inner

	// Open: the closing border belongs to the live piece still below.
	rendered, _ := style.MultiSectionBoxOpts(m.styleSections(m.sections[:n]), m.boxOpts(true))
	m.continued = true
	m.sections = m.sections[n:]
	// Re-index: a late message addressed to a committed section must miss the
	// map entirely rather than land on whichever section inherited its index.
	m.sectionByID = make(map[string]int, len(m.sections))
	for i, sec := range m.sections {
		m.sectionByID[sec.id] = i
	}
	return m, tea.Println(rendered)
}

func renderSection(sec section, sp spinner.Model) string {
	// Progress-bar rows line up on a common column regardless of label
	// length — otherwise mixed label widths give bars different horizontal
	// offsets, which reads as broken.
	maxLabel := 0
	for _, id := range sec.rowOrder {
		if row := sec.rows[id]; row.Progress != nil {
			if w := lipgloss.Width(row.Label); w > maxLabel {
				maxLabel = w
			}
		}
	}

	lines := make([]string, 0, len(sec.rowOrder)+1)
	for _, id := range sec.rowOrder {
		row := sec.rows[id]
		if row.Progress != nil {
			pad := strings.Repeat(" ", maxLabel-lipgloss.Width(row.Label))
			lines = append(lines, fmt.Sprintf("%s%s  %s", row.Label, pad, style.ProgressBar(*row.Progress)))
			continue
		}
		line := rowIcon(row.State, sp) + row.Label
		if row.Detail != "" {
			line += "  " + row.Detail
		}
		lines = append(lines, line)
	}

	if sec.finished {
		if sec.plain {
			lines = append(lines, sec.summary)
		} else {
			icon := style.TickIcon.Render()
			if sec.failed {
				icon = style.CrossIcon.Render()
			}
			lines = append(lines, icon+sec.summary)
		}
	}

	return strings.Join(lines, "\n")
}

func rowIcon(state RowState, sp spinner.Model) string {
	switch state {
	case Pending:
		return style.TodoIcon.Render()
	case Active:
		return sp.View() + " "
	case Done:
		return style.TickIcon.Render()
	case Failed:
		return style.CrossIcon.Render()
	default:
		return sp.View() + " "
	}
}
