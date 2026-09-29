package tablepageselect

import (
	"context"
	"fmt"
	"strconv"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tuikeys "github.com/argus-labs/world-engine/cli/internal/tui/kit/keys"
	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

// Model represents the UI state for paginated selection with a table and prompt.
type Model struct {
	Prompt     string
	Columns    []string   // excludes the blank rank column
	Rows       [][]string // excludes the rank cell; rank is auto-generated
	Cursor     int        // global selected index across all rows
	NumericBuf string
	Ctx        context.Context
	Aborted    bool
	Canceled   bool
	finalized  bool

	// ExtraKeys close the picker and report their action through HotkeyAction.
	ExtraKeys    []ExtraKey
	HotkeyAction string

	legendKeys pickerLegendKeys
	legend     tuikeys.Legend

	PageSize int
	tbl      table.Model
}

const (
	// MaxColumnWidth caps how wide a non-rank column can be.
	MaxColumnWidth = 100
	// MinTableHeightRows ensures the viewport is never too small to render properly.
	MinTableHeightRows = 2
	// DefaultPageSize limits visible rows per page.
	DefaultPageSize = 10
)

// InitialModel creates a new Model with the given columns, rows, context and default cursor.
func InitialModel(ctx context.Context, prompt string, columns []string, rows [][]string, defaultIndex int) *Model {
	if defaultIndex < 0 {
		defaultIndex = 0
	}
	if defaultIndex >= len(rows) && len(rows) > 0 {
		defaultIndex = len(rows) - 1
	}

	// Compute column widths from content with sane caps (based on all rows).
	widths := computeColumnWidths(columns, rows, MaxColumnWidth)

	// Build table columns: blank rank header first.
	tblCols := make([]table.Column, 0, len(columns)+1)
	tblCols = append(tblCols, table.Column{Title: "", Width: widths[0]})
	for i, c := range columns {
		tblCols = append(tblCols, table.Column{Title: c, Width: widths[i+1]})
	}

	m := &Model{
		Prompt:     prompt,
		Columns:    columns,
		Rows:       rows,
		Cursor:     defaultIndex,
		Ctx:        ctx,
		PageSize:   DefaultPageSize,
		legendKeys: defaultPickerLegendKeys(),
		legend:     tuikeys.NewLegend(),
	}

	// Create table with columns and initial page rows.
	t := table.New(
		table.WithColumns(tblCols),
		table.WithFocused(true),
	)
	// Style similar to table_select.
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(style.DarkOrange)).
		BorderBottom(true).
		Bold(false).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Top).
		Foreground(lipgloss.Color(style.Yellow))
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color(style.Yellow)).
		Bold(true)
	t.SetStyles(s)

	m.tbl = t
	m.refreshTableRows()

	return m
}

func (m *Model) Init() tea.Cmd { return nil }

// Update handles user input and updates the model state accordingly.
//
//nolint:nestif // Common complexity in bubbletea update.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	select {
	case <-m.Ctx.Done():
		return m, tea.Quit
	default:
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if m.Cursor > 0 {
				m.Cursor--
				m.refreshTableRows()
			}
			m.NumericBuf = ""
		case "down", "j":
			if m.Cursor < len(m.Rows)-1 {
				m.Cursor++
				m.refreshTableRows()
			}
			m.NumericBuf = ""
		case "left", "h":
			m.pageLeft()
			m.NumericBuf = ""
		case "right", "l":
			m.pageRight()
			m.NumericBuf = ""
		case "enter":
			m.finalized = true
			return m, tea.Quit
		case "esc":
			m.Aborted = true
			return m, tea.Quit
		case "ctrl+c":
			m.Canceled = true
			return m, tea.Quit
		case "backspace":
			if len(m.NumericBuf) > 0 {
				m.NumericBuf = m.NumericBuf[:len(m.NumericBuf)-1]
				m.applyNumericBuf()
			}
		default:
			// Extra hotkey lookup (e.g. "r" → "reload"). Wins before the
			// digit-handling branch so single-letter hotkeys aren't swallowed.
			if m.legend.HandleToggle(keyMsg, len(m.ExtraKeys) > 0) {
				return m, nil
			}
			if action, ok := m.matchExtraKey(keyMsg); ok {
				m.HotkeyAction = action
				m.finalized = true
				return m, tea.Quit
			}
			// digits handling.
			if keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) > 0 {
				r := keyMsg.Runes[0]
				if unicode.IsDigit(r) {
					m.NumericBuf += string(r)
					m.applyNumericBuf()
				}
			}
		}
	}
	return m, nil
}

// View renders the current state of the list and the numeric prompt.
func (m *Model) View() string {
	// Wrap table in an orange bordered panel similar to the example.
	panel := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(style.Orange)).
		Padding(0)
	s := panel.Render(m.tbl.View())

	// Prompt line.
	placeholder := strconv.Itoa(m.Cursor + 1)
	if m.NumericBuf == "" {
		// Greyed placeholder like textinput.
		grey := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(placeholder)
		s += fmt.Sprintf("\n%s: %s\n", m.Prompt, grey)
	} else {
		s += fmt.Sprintf("\n%s: %s\n", m.Prompt, m.NumericBuf)
	}
	pg, total := m.currentPageIndex()+1, m.totalPages()
	s += fmt.Sprintf("\n%s (Pg %d/%d)\n", m.legend.View(m.primaryBindings(), m.secondaryBindings()), pg, total)
	return s
}

func (m *Model) matchExtraKey(msg tea.KeyMsg) (string, bool) {
	for _, extra := range m.ExtraKeys {
		if key.Matches(msg, extra.Binding) {
			return extra.Action, true
		}
	}
	return "", false
}

func (m *Model) primaryBindings() []key.Binding {
	primary := []key.Binding{m.legendKeys.Nav, m.legendKeys.Select}
	if len(m.ExtraKeys) == 0 {
		return append(primary, m.legendKeys.Quit)
	}
	for _, extra := range m.ExtraKeys {
		if !extra.Secondary {
			primary = append(primary, extra.Binding)
		}
	}
	return primary
}

func (m *Model) secondaryBindings() []key.Binding {
	if len(m.ExtraKeys) == 0 {
		return nil
	}
	secondary := make([]key.Binding, 0, len(m.ExtraKeys)+1)
	for _, extra := range m.ExtraKeys {
		if extra.Secondary {
			secondary = append(secondary, extra.Binding)
		}
	}
	return append(secondary, m.legendKeys.Quit)
}

// applyNumericBuf parses NumericBuf and moves the cursor accordingly.
func (m *Model) applyNumericBuf() {
	if len(m.Rows) == 0 {
		return
	}
	if m.NumericBuf == "" {
		return
	}
	// Strip leading zeros but keep at least one digit.
	for len(m.NumericBuf) > 1 && m.NumericBuf[0] == '0' {
		m.NumericBuf = m.NumericBuf[1:]
	}
	if n, err := strconv.Atoi(m.NumericBuf); err == nil {
		if n < 1 {
			n = 1
		}
		if n > len(m.Rows) {
			n = len(m.Rows)
		}
		m.Cursor = n - 1
		m.refreshTableRows()
	}
}

// pageLeft moves the selection one full page to the left (previous page), preserving row offset.
func (m *Model) pageLeft() {
	if len(m.Rows) == 0 {
		return
	}
	within := m.Cursor % m.PageSize
	start := m.currentPageStart()
	if start == 0 {
		// Already at first page.
		m.refreshTableRows()
		return
	}
	newStart := max(start-m.PageSize, 0)
	pageLen := minInt(m.PageSize, len(m.Rows)-newStart)
	if within >= pageLen {
		within = pageLen - 1
	}
	m.Cursor = newStart + within
	m.refreshTableRows()
}

// pageRight moves the selection one full page to the right (next page), preserving row offset.
func (m *Model) pageRight() {
	if len(m.Rows) == 0 {
		return
	}
	within := m.Cursor % m.PageSize
	start := m.currentPageStart()
	newStart := start + m.PageSize
	if newStart >= len(m.Rows) {
		// Already at last page.
		m.refreshTableRows()
		return
	}
	pageLen := minInt(m.PageSize, len(m.Rows)-newStart)
	if within >= pageLen {
		within = pageLen - 1
	}
	m.Cursor = newStart + within
	m.refreshTableRows()
}

// currentPageStart returns the starting index of the current page.
func (m *Model) currentPageStart() int {
	if m.PageSize <= 0 {
		return 0
	}
	return (m.Cursor / m.PageSize) * m.PageSize
}

// currentPageIndex returns the current page index (0-based).
func (m *Model) currentPageIndex() int {
	if m.PageSize <= 0 {
		return 0
	}
	return m.Cursor / m.PageSize
}

// totalPages returns the total number of pages (>= 1 when rows exist, else 1).
func (m *Model) totalPages() int {
	if len(m.Rows) == 0 {
		return 1
	}
	return ((len(m.Rows) - 1) / m.PageSize) + 1
}

// refreshTableRows updates the visible rows for the current page and syncs the table cursor.
func (m *Model) refreshTableRows() {
	if len(m.Rows) == 0 {
		m.tbl.SetRows(nil)
		return
	}
	// Clamp cursor within bounds.
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Rows) {
		m.Cursor = len(m.Rows) - 1
	}
	start := m.currentPageStart()
	end := minInt(start+m.PageSize, len(m.Rows))
	visible := make([]table.Row, 0, end-start)
	for i := start; i < end; i++ {
		row := []string{strconv.Itoa(i + 1)}
		row = append(row, m.Rows[i]...)
		visible = append(visible, row)
	}
	m.tbl.SetRows(visible)
	// Height = clamp(len(visible), MinTableHeightRows, PageSize) + 2.
	height := max(len(visible), MinTableHeightRows)
	m.tbl.SetHeight(height + 2)
	// Set table cursor to index within the page.
	m.tbl.SetCursor(m.Cursor - start)
}

// computeColumnWidths returns widths including the first rank column at index 0.
func computeColumnWidths(columns []string, rows [][]string, maxColWidth int) []int {
	widths := make([]int, len(columns)+1)
	// Rank column: width to fit the max row number + 1 space.
	digits := max(len(strconv.Itoa(len(rows))), 2)
	widths[0] = digits + 1

	for ci, title := range columns {
		maxw := lipgloss.Width(title) + 1 // min: title + 1
		for _, r := range rows {
			if ci < len(r) {
				w := lipgloss.Width(r[ci])
				if w > maxw {
					maxw = w
				}
			}
		}
		if maxw > maxColWidth {
			maxw = maxColWidth
		}
		widths[ci+1] = maxw
	}
	return widths
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
