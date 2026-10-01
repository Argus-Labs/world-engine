package tableselect

import (
	"context"
	"fmt"
	"strconv"
	"unicode"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

// Model represents the UI state for numeric selection with a table and prompt.
type Model struct {
	Prompt     string
	Columns    []string   // excludes the blank rank column
	Rows       [][]string // excludes the rank cell; rank is auto-generated
	Cursor     int
	NumericBuf string
	Ctx        context.Context
	Aborted    bool
	finalized  bool

	tbl table.Model
}

const (
	// MaxColumnWidth caps how wide a non-rank column can be.
	MaxColumnWidth = 30
	// MinTableHeightRows ensures the viewport is never too small to render properly.
	MinTableHeightRows = 2
)

// InitialModel creates a new Model with the given columns, rows, context and default cursor.
func InitialModel(ctx context.Context, prompt string, columns []string, rows [][]string, defaultIndex int) *Model {
	if defaultIndex < 0 {
		defaultIndex = 0
	}
	if defaultIndex >= len(rows) && len(rows) > 0 {
		defaultIndex = len(rows) - 1
	}

	// Compute column widths from content with sane caps.
	widths := computeColumnWidths(columns, rows, MaxColumnWidth)

	// Build table columns: blank rank header first
	tblCols := make([]table.Column, 0, len(columns)+1)
	tblCols = append(tblCols, table.Column{Title: "", Width: widths[0]})
	for i, c := range columns {
		tblCols = append(tblCols, table.Column{Title: c, Width: widths[i+1]})
	}

	// Build table rows: rank + provided cells
	tblRows := make([]table.Row, 0, len(rows))
	for i, r := range rows {
		row := []string{strconv.Itoa(i + 1)}
		row = append(row, r...)
		tblRows = append(tblRows, row)
	}

	t := table.New(
		table.WithColumns(tblCols),
		table.WithRows(tblRows),
		table.WithFocused(true),
	)
	// style similar to example
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
	// Height = clamp(len(rows), MinTableHeightRows, MaxTableHeightRows)
	height := max(len(tblRows), MinTableHeightRows)

	t.SetHeight(height + 2)
	// Let table compute width from column widths to avoid extra right padding
	// (no explicit SetWidth)

	t.SetCursor(defaultIndex)

	m := &Model{
		Prompt:  prompt,
		Columns: columns,
		Rows:    rows,
		Cursor:  defaultIndex,
		Ctx:     ctx,
		tbl:     t,
	}
	return m
}

func (m *Model) Init() tea.Cmd { return nil }

// Update handles user input and updates the model state accordingly.
//
//nolint:nestif // Common complexity in bubbletea update
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
			}
			m.tbl.SetCursor(m.Cursor)
			m.NumericBuf = ""
		case "down", "j":
			if m.Cursor < len(m.Rows)-1 {
				m.Cursor++
			}
			m.tbl.SetCursor(m.Cursor)
			m.NumericBuf = ""
		case "enter":
			m.finalized = true
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.Aborted = true
			return m, tea.Quit
		case "backspace":
			if len(m.NumericBuf) > 0 {
				m.NumericBuf = m.NumericBuf[:len(m.NumericBuf)-1]
				m.applyNumericBuf()
				m.tbl.SetCursor(m.Cursor)
			}
		default:
			// digits handling
			if keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) > 0 {
				r := keyMsg.Runes[0]
				if unicode.IsDigit(r) {
					m.NumericBuf += string(r)
					m.applyNumericBuf()
					m.tbl.SetCursor(m.Cursor)
				}
			}
		}
	}
	return m, nil
}

// View renders the current state of the list and the numeric prompt.
func (m *Model) View() string {
	// Wrap table in an orange bordered panel similar to the example
	panel := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(style.Orange)).
		Padding(0)
	s := panel.Render(m.tbl.View())

	// Prompt line
	placeholder := strconv.Itoa(m.Cursor + 1)
	if m.NumericBuf == "" {
		// greyed placeholder like textinput
		grey := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(placeholder)
		s += fmt.Sprintf("\n%s: %s\n", m.Prompt, grey)
	} else {
		s += fmt.Sprintf("\n%s: %s\n", m.Prompt, m.NumericBuf)
	}
	s += "\n(Type a number, up/down arrows, esc to quit)\n"
	return s
}

// applyNumericBuf parses NumericBuf and moves the cursor accordingly.

func (m *Model) applyNumericBuf() {
	if len(m.Rows) == 0 {
		return
	}
	if m.NumericBuf == "" {
		return
	}
	// strip leading zeros but keep at least one digit
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
	}
}

// computeColumnWidths returns widths including the first rank column at index 0.
func computeColumnWidths(columns []string, rows [][]string, maxColWidth int) []int {
	widths := make([]int, len(columns)+1)
	// rank column: width to fit the max row number + 1 space
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
