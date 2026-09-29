package phasebox

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

// step drives one message through Update and returns the new model plus the
// command it produced, keeping the tea.Model type assertion out of the tests.
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	require.True(t, ok, "Update must return a phasebox.Model")
	return got, cmd
}

// finished opens a section, adds a row, and collapses it.
func finished(t *testing.T, m Model, id, title string) Model {
	t.Helper()
	m, _ = step(t, m, newSectionMsg{id: id, title: title})
	m, _ = step(t, m, rowMsg{section: id, id: "r", label: "work", state: Done})
	m, _ = step(t, m, collapseMsg{section: id, summary: "done"})
	return m
}

func TestFlushCommitsFinishedSectionsToScrollback(t *testing.T) {
	t.Parallel()

	// Height 5 is too short to hold both sections, forcing a flush.
	m := newModel(nil)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 5})
	m = finished(t, m, "1", "Image Pull")
	require.Len(t, m.sections, 1, "a finished section stays live until the frame runs out of room")

	m, cmd := step(t, m, newSectionMsg{id: "2", title: "Build"})

	require.NotNil(t, cmd, "an overflowing frame must commit finished sections via tea.Println")
	require.Len(t, m.sections, 1, "only the in-flight section stays in the live frame")
	require.Equal(t, "2", m.sections[0].id)
	require.True(t, m.continued, "the live piece must open with a divider, not a second top corner")
}

// Regression (reported on a real `world start`): sizing the box from the
// terminal instead of its content left a wall of dead space to the right.
// While everything still fits, nothing is committed to scrollback, so the box
// is free to shrink to its content exactly as a single-frame box would.
func TestBoxHugsContentWhileTheFrameFits(t *testing.T) {
	t.Parallel()

	const termWidth = 120

	m := newModel(nil)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: termWidth, Height: 40})
	m = finished(t, m, "1", "Build")
	m, cmd := step(t, m, newSectionMsg{id: "2", title: "Cluster"})

	require.Nil(t, cmd, "nothing should be committed to scrollback while the frame fits")
	require.False(t, m.continued)
	require.Equal(t, 0, m.innerWidth, "the width stays unfrozen, so a later section can still widen the box")

	widest := 0
	for line := range strings.SplitSeq(m.View(), "\n") {
		widest = max(widest, lipgloss.Width(line))
	}
	require.Less(t, widest, termWidth, "a box that fits must hug its content, not fill the terminal")
}

// Regression: flushing shifts the remaining sections' indices, so a stale id
// left in sectionByID would point at whichever section inherited its slot and
// silently corrupt it.
func TestFlushDropsIDsOfCommittedSections(t *testing.T) {
	t.Parallel()

	m := newModel(nil)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 5})
	m = finished(t, m, "1", "Image Pull")
	m, _ = step(t, m, newSectionMsg{id: "2", title: "Build"})
	require.True(t, m.continued, "precondition: section 1 must have been committed")

	_, ok := m.sectionByID["1"]
	require.False(t, ok, "a committed section's id must be dropped from the index")
	require.Equal(t, 0, m.sectionByID["2"], "the live section must be re-indexed to its new slot")

	m, _ = step(t, m, rowMsg{section: "1", id: "late", label: "late row", state: Active})
	require.Empty(t, m.sections[0].rowOrder, "a late row for a committed section must not land on the live one")
}

// The bug this guards: bubbletea drops lines off the TOP of a frame taller
// than the screen, and a live-only frame never reaches scrollback, so early
// sections would be lost entirely on a short terminal.
func TestLiveFrameStaysBoundedAcrossManySections(t *testing.T) {
	t.Parallel()

	const (
		height   = 24
		sections = 20 // unflushed these render ~61 lines, far past the screen
	)

	m := newModel(nil)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: height})
	for i := range sections {
		m = finished(t, m, strconv.Itoa(i), "Section "+strconv.Itoa(i))
	}

	require.LessOrEqual(t, strings.Count(m.View(), "\n"), height,
		"the live frame must stay within the terminal height however many sections ran")
}

func TestViewClampsToTerminalWidth(t *testing.T) {
	t.Parallel()

	m := newModel(nil)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 60, Height: 24})
	m, _ = step(t, m, newSectionMsg{id: "1", title: "Cluster"})
	m, _ = step(t, m, rowMsg{
		section: "1", id: "r",
		label: strings.Repeat("very-long-detail ", 20), state: Active,
	})

	for line := range strings.SplitSeq(m.View(), "\n") {
		require.LessOrEqual(t, lipgloss.Width(line), 60,
			"no rendered line may exceed the terminal width, or the renderer eats the border")
	}
}
