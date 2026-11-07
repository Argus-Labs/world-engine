package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
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

// When --directory is supplied with an over-length basename (the c108684e
// regression), the name the user types into the fix-up input must be applied:
// targetDir is rebased onto the validated name so cloning and world.toml
// generation use a valid DNS-1123 label. Before the fix the typed name was
// silently discarded and setup failed at the very end with "project is too
// long (max 63 characters)".
func TestHandleNameInputKey_OverLengthDirRebasesOntoTypedName(t *testing.T) {
	t.Parallel()

	longName := strings.Repeat("a", 64)
	m := NewWorldSetupModel("/tmp/"+longName, "DEV", "")
	require.Equal(t, projectNameErrMsg, m.nameErr)
	require.True(t, m.projectNameInput.Focused())

	m.projectNameInput.SetValue("my-game")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, ok := next.(WorldSetupModel)
	require.True(t, ok, "Update should return a WorldSetupModel")

	require.Empty(t, mm.nameErr, "name error should be cleared after valid name")
	require.False(t, mm.projectNameInput.Focused(), "input should be blurred")

	// The typed name must be applied to targetDir, not discarded.
	require.Equal(t, "/tmp/my-game", mm.targetDir,
		"targetDir should be rebased onto the validated name")
	require.Equal(t, "/tmp/my-game", mm.targetDirectory(),
		"targetDirectory() should return the rebased path")
	require.Equal(t, "/tmp/my-game", mm.displayDir(),
		"displayDir() should return the rebased path")

	// The project name that InstantiateTemplate would derive from the target
	// directory must be a canonical DNS-1123 label (the root cause of the
	// end-of-flow WriteProject failure).
	require.True(t, dnslabel.IsCanonical(filepath.Base(mm.targetDirectory())),
		"filepath.Base(targetDirectory()) must be canonical so world.toml validation passes")
}

// When --directory is supplied with a charset-invalid basename (e.g.
// underscore), the fix-up input's typed name must also be applied. This case
// has failed deterministically since c13cd880 routed the scaffold through the
// user-supplied directory; the fix makes the input live up to its promise.
func TestHandleNameInputKey_InvalidCharsetDirRebasesOntoTypedName(t *testing.T) {
	t.Parallel()

	m := NewWorldSetupModel("/tmp/my_game", "DEV", "")
	require.Equal(t, projectNameErrMsg, m.nameErr)
	require.True(t, m.projectNameInput.Focused())

	m.projectNameInput.SetValue("my-game")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, ok := next.(WorldSetupModel)
	require.True(t, ok, "Update should return a WorldSetupModel")

	require.Empty(t, mm.nameErr)
	require.Equal(t, "/tmp/my-game", mm.targetDir,
		"targetDir should be rebased onto the validated name")
	require.True(t, dnslabel.IsCanonical(filepath.Base(mm.targetDirectory())),
		"derived project name must be canonical")
}

// A trailing slash on the --directory argument must not confuse the rebase:
// the path is cleaned before extracting the parent, so
// "/tmp/my_game/" rebases to "/tmp/my-game", not "/tmp/my_game/my-game".
func TestHandleNameInputKey_TrailingSlashDirRebasesCorrectly(t *testing.T) {
	t.Parallel()

	m := NewWorldSetupModel("/tmp/my_game/", "DEV", "")
	require.Equal(t, projectNameErrMsg, m.nameErr)

	m.projectNameInput.SetValue("my-game")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, ok := next.(WorldSetupModel)
	require.True(t, ok)

	require.Equal(t, "/tmp/my-game", mm.targetDir,
		"trailing slash should be normalized before rebasing")
	require.True(t, dnslabel.IsCanonical(filepath.Base(mm.targetDirectory())))
}

// A relative --directory argument (no path separator) is rebased relative to
// the current directory: "my_game" becomes "my-game", not "./my-game".
func TestHandleNameInputKey_RelativeDirRebasesOntoTypedName(t *testing.T) {
	t.Parallel()

	m := NewWorldSetupModel("my_game", "DEV", "")
	require.Equal(t, projectNameErrMsg, m.nameErr)

	m.projectNameInput.SetValue("my-game")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, ok := next.(WorldSetupModel)
	require.True(t, ok)

	require.Equal(t, "my-game", mm.targetDir,
		"relative dir should rebase to a relative path with the validated name")
	require.True(t, dnslabel.IsCanonical(filepath.Base(mm.targetDirectory())))
}

// When no --directory is supplied, the fix-up input must NOT set targetDir:
// targetDirectory() falls back to the input value, and the fix must not
// change that behavior. This is the no-regression guard for the common path.
func TestHandleNameInputKey_NoDirDoesNotSetTargetDir(t *testing.T) {
	t.Parallel()

	m := NewWorldSetupModel("", "DEV", "")
	require.True(t, m.projectNameInput.Focused())
	require.Empty(t, m.targetDir)

	m.projectNameInput.SetValue("my-game")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, ok := next.(WorldSetupModel)
	require.True(t, ok)

	require.Empty(t, mm.targetDir,
		"targetDir must stay empty when no --directory was supplied")
	require.Equal(t, "my-game", mm.targetDirectory(),
		"targetDirectory() should fall back to the input value")
	require.Equal(t, "./my-game", mm.displayDir(),
		"displayDir() should fall back to ./<input value>")
}
