package tableview

import (
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stripANSI removes all ANSI escape codes from a string.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func TestInitialModel_BuildsTable(t *testing.T) {
	t.Parallel()

	columns := []string{"Name", "Type"}
	rows := [][]string{{"one", "alpha"}, {"two", "beta"}}
	m := InitialModel(columns, rows)
	require.NotNil(t, m)
	assert.Equal(t, columns, m.Columns)
	assert.Equal(t, rows, m.Rows)

	view := m.View()
	assert.Contains(t, view, "Name")
	assert.Contains(t, view, "Type")
}

func TestModel_QuitsOnTick(t *testing.T) {
	t.Parallel()
	m := InitialModel([]string{"A"}, [][]string{{"x"}})

	cmd := m.Init()
	require.NotNil(t, cmd)

	ret, _ := m.Update(doneMsg{})
	_, ok := ret.(*Model)
	assert.True(t, ok)

	// Ensure unknown msg doesn't panic
	assert.NotPanics(t, func() {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes})
	})
}

func TestRun_Succeeds(t *testing.T) {
	t.Parallel()

	err := Run([]string{"Name", "Type"}, [][]string{{"one", "alpha"}})
	assert.NoError(t, err)
}

func TestRun_NoColumns(t *testing.T) {
	t.Parallel()

	err := Run([]string{}, [][]string{{}})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoColumns)
}

// captureStdout captures [os.Stdout] output produced by fn and returns it as a string.
var stdoutMu sync.Mutex

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	stdoutMu.Lock()
	defer stdoutMu.Unlock()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w //nolint:reassign // capturing stdout in tests

	fn()

	_ = w.Close()
	os.Stdout = orig //nolint:reassign // restore stdout
	buf, _ := io.ReadAll(r)
	_ = r.Close()
	return string(buf)
}

func Test_printPlainTable_PrintsHeaderDividerAndRows(t *testing.T) {
	// Cannot run in parallel due to stdout capture

	columns := []string{"Name", "Type"}
	rows := [][]string{{"one", "alpha"}, {"two", "beta"}}

	out := captureStdout(t, func() { printPlainTable(columns, rows) })
	plainOut := stripANSI(out)

	// Header text should be present (styled) and a divider line printed.
	assert.Contains(t, plainOut, "Name")
	assert.Contains(t, plainOut, "Type")

	// Verify divider length matches computeColumnWidths logic.
	widths := computeColumnWidths(columns, rows, MaxColumnWidth)
	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	if len(columns) > 1 {
		totalWidth += (len(columns) - 1) * 2
	}
	divider := strings.Repeat("-", totalWidth)
	// Find a line that equals the divider.
	hasDivider := slices.Contains(strings.Split(plainOut, "\n"), divider)
	assert.True(t, hasDivider, "expected divider of length %d not found in output:\n%s", totalWidth, plainOut)

	// Rows content should be printed on their own lines.
	assert.Contains(t, plainOut, "one")
	assert.Contains(t, plainOut, "alpha")
	assert.Contains(t, plainOut, "two")
	assert.Contains(t, plainOut, "beta")

	// Log the rendered table for visual inspection when running tests with -v.
	t.Logf("\nRendered table (fallback):\n%s", plainOut)
}

func Test_printPlainTable_AllowsMissingCells(t *testing.T) {
	// Cannot run in parallel due to stdout capture

	columns := []string{"ID", "Description", "Owner"}
	rows := [][]string{{"1", "short"}, {"2"}}

	out := captureStdout(t, func() { printPlainTable(columns, rows) })
	plainOut := stripANSI(out)

	// Header should include all columns.
	assert.Contains(t, plainOut, "ID")
	assert.Contains(t, plainOut, "Description")
	assert.Contains(t, plainOut, "Owner")

	// Rows should still render even if some cells are missing.
	assert.Contains(t, plainOut, "1")
	assert.Contains(t, plainOut, "short")
	assert.Contains(t, plainOut, "2")

	// Log for visual inspection.
	t.Logf("\nRendered table with missing cells (fallback):\n%s", plainOut)
}

func Test_printPlainTable_TruncatesExtraCellsWhenColumnsMissing(t *testing.T) {
	// Cannot run in parallel due to stdout capture

	// Only one column provided, but rows have more cells.
	columns := []string{"Name"}
	rows := [][]string{{"one", "alpha"}, {"two", "beta"}}

	out := captureStdout(t, func() { printPlainTable(columns, rows) })
	plainOut := stripANSI(out)

	// Header present
	assert.Contains(t, plainOut, "Name")
	// Should include first cell of each row
	assert.Contains(t, plainOut, "one")
	assert.Contains(t, plainOut, "two")
	// Should NOT include cells beyond provided columns
	assert.NotContains(t, plainOut, "alpha")
	assert.NotContains(t, plainOut, "beta")

	t.Logf("\nRendered table with fewer columns than row cells (fallback):\n%s", plainOut)
}
