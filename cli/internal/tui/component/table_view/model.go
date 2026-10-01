package tableview

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

// Model represents the UI state for a read-only table view.
type Model struct {
	Columns []string   // headers
	Rows    [][]string // cells per row

	tbl table.Model
}

const (
	// MaxColumnWidth caps how wide a column can be.
	MaxColumnWidth = 100
	// MinTableHeightRows ensures the viewport is never too small to render properly.
	MinTableHeightRows = 2
)

// InitialModel creates a new Model with the given columns and rows.
func InitialModel(columns []string, rows [][]string) *Model {
	// Compute column widths from content with sane caps.
	widths := computeColumnWidths(columns, rows, MaxColumnWidth)

	// Build table columns using provided headers.
	tblCols := make([]table.Column, 0, len(columns))
	for i, c := range columns {
		title := centerText(c, widths[i])
		tblCols = append(tblCols, table.Column{Title: title, Width: widths[i]})
	}

	// Build table rows from provided cells.
	tblRows := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		row := make([]string, 0, len(columns))
		row = append(row, r...)
		tblRows = append(tblRows, row)
	}

	t := table.New(
		table.WithColumns(tblCols),
		table.WithRows(tblRows),
	)

	// Style similar to table_select
	ts := table.DefaultStyles()
	ts.Header = ts.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(style.DarkOrange)).
		BorderBottom(true).
		Bold(true).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Top).
		Foreground(lipgloss.Color(style.Yellow))

	// default table style is pink on selected, so we need to unset it
	ts.Selected = lipgloss.NewStyle().
		UnsetForeground().
		UnsetBackground().
		UnsetBold().
		UnsetItalic()

	t.SetStyles(ts)

	// Height = clamp(len(rows), MinTableHeightRows, ...)
	height := max(len(tblRows), MinTableHeightRows)

	if len(rows) == 1 {
		height = 1
	}
	t.SetHeight(height + 2)

	m := &Model{
		Columns: columns,
		Rows:    rows,
		tbl:     t,
	}
	return m
}

// doneMsg signals the program to quit after the initial render.
type doneMsg struct{}

// Init initializes the bubbletea model and schedules a quit on the next tick to allow one render.
func (m *Model) Init() tea.Cmd {
	return tea.Tick(0, func(time.Time) tea.Msg { return doneMsg{} })
}

// Update exits immediately on context cancellation or after the first tick.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(doneMsg); ok {
		return m, tea.Quit
	}
	return m, nil
}

// View renders the current state of the table.
func (m *Model) View() string {
	panel := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(style.Orange)).
		Padding(0)
	s := panel.Render(m.tbl.View())
	// No hints; just the table view.
	return fmt.Sprintf("%s\n", s)
}

// computeColumnWidths returns widths for provided columns.
func computeColumnWidths(columns []string, rows [][]string, maxColWidth int) []int {
	widths := make([]int, len(columns))
	for ci, title := range columns {
		maxw := lipgloss.Width(title)
		for _, r := range rows {
			if ci < len(r) {
				if tw := lipgloss.Width(r[ci]); tw > maxw {
					maxw = tw
				}
			}
		}
		if maxw > maxColWidth {
			maxw = maxColWidth
		}

		widths[ci] = maxw
	}
	return widths
}

func centerText(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	gap := width - w
	left := gap / 2
	right := gap - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}
