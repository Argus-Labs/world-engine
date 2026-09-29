package tableview

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

var (
	// ErrNoColumns indicates there were no columns to display.
	ErrNoColumns = eris.New("no columns to display")
)

// Run executes the read-only table UI.
func Run(
	columns []string,
	rows [][]string,
) error {
	if len(columns) == 0 {
		return ErrNoColumns
	}

	prog := program.NewTeaProgram(InitialModel(columns, rows))
	_, err := prog.Run()
	if err != nil {
		// Fallback: if Bubble Tea UI fails, print a plain table instead.
		printPlainTable(columns, rows)
	}
	return nil
}

// printPlainTable renders a simple, non-interactive table to stdout using the printer package.
// This is used as a graceful fallback when the TUI cannot be rendered.
func printPlainTable(columns []string, rows [][]string) {
	widths := computeColumnWidths(columns, rows, MaxColumnWidth)

	// Helper to right-pad a cell to the target display width.
	pad := func(s string, width int) string {
		w := lipgloss.Width(s)
		if w >= width {
			return s
		}
		return s + strings.Repeat(" ", width-w)
	}

	// Header line.
	headerCells := make([]string, len(columns))
	for i, col := range columns {
		headerCells[i] = pad(col, widths[i])
	}
	printer.Headerln(strings.Join(headerCells, "  "))

	// Divider matching total table width.
	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	if len(columns) > 1 {
		totalWidth += (len(columns) - 1) * 2 // two spaces between columns
	}
	printer.SectionDivider("-", totalWidth)

	// Rows.
	for _, r := range rows {
		cells := make([]string, len(columns))
		for ci := range columns {
			var val string
			if ci < len(r) {
				val = r[ci]
			}
			cells[ci] = pad(val, widths[ci])
		}
		printer.Infoln(strings.Join(cells, "  "))
	}
}
