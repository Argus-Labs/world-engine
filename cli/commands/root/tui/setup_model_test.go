package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// projectNameErrMsg must mention the 63-character limit so a user who hits the
// length ceiling gets actionable guidance instead of a character-only message.
func TestProjectNameErrMsg_MentionsLengthLimit(t *testing.T) {
	t.Parallel()

	require.Contains(t, projectNameErrMsg, "63")
}

// A directory whose basename exceeds the DNS-1123 label limit (63 chars) must
// set nameErr so the setup TUI redirects the user back to the name input rather
// than accepting the name and scaffolding around it.
func TestNewWorldSetupModel_RejectsOverLengthDirName(t *testing.T) {
	t.Parallel()

	longName := strings.Repeat("a", 64)
	m := NewWorldSetupModel("/tmp/"+longName, "DEV", "")
	require.Equal(t, projectNameErrMsg, m.nameErr)
	require.True(t, m.projectNameInput.Focused(),
		"input should be focused so the user can fix the over-length name")
}

// A directory whose basename is exactly 63 characters is the boundary case and
// must be accepted (the limit is <= 63, not < 63).
func TestNewWorldSetupModel_AcceptsMaxLengthDirName(t *testing.T) {
	t.Parallel()

	maxName := strings.Repeat("a", 63)
	m := NewWorldSetupModel("/tmp/"+maxName, "DEV", "")
	require.Empty(t, m.nameErr)
	require.False(t, m.projectNameInput.Focused(),
		"a valid arg name should skip the input step")
}

// A short, canonical directory name is accepted (no regression in the common case).
func TestNewWorldSetupModel_AcceptsShortName(t *testing.T) {
	t.Parallel()

	m := NewWorldSetupModel("/tmp/my-game", "DEV", "")
	require.Empty(t, m.nameErr)
	require.False(t, m.projectNameInput.Focused())
}

// Typing a 64-char name and pressing Enter must NOT complete the step; the name
// stays in the input with the error shown. Before the fix, the over-length name
// would have been accepted and the step completed.
func TestHandleNameInputKey_RejectsOverLengthName(t *testing.T) {
	t.Parallel()

	m := NewWorldSetupModel("", "DEV", "")
	require.True(t, m.projectNameInput.Focused())

	longName := strings.Repeat("a", 64)
	m.projectNameInput.SetValue(longName)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, ok := next.(WorldSetupModel)
	require.True(t, ok, "Update should return a WorldSetupModel")
	require.Equal(t, projectNameErrMsg, mm.nameErr)
	require.True(t, mm.projectNameInput.Focused(),
		"input should stay focused so the user can fix the name")
}

// Typing a 63-char name and pressing Enter completes the step (boundary acceptance
// through the interactive path, not just the directory-argument path).
func TestHandleNameInputKey_AcceptsMaxLengthName(t *testing.T) {
	t.Parallel()

	m := NewWorldSetupModel("", "DEV", "")
	require.True(t, m.projectNameInput.Focused())

	maxName := strings.Repeat("a", 63)
	m.projectNameInput.SetValue(maxName)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, ok := next.(WorldSetupModel)
	require.True(t, ok, "Update should return a WorldSetupModel")
	require.Empty(t, mm.nameErr)
	require.False(t, mm.projectNameInput.Focused(),
		"input should be blurred after accepting the name")
}
