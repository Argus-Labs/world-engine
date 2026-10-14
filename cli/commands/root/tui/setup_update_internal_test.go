package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
)

// enterNameAndComplete sets the project name input to name and sends Enter,
// returning the updated model. It mirrors what a user does in the name step.
func enterNameAndComplete(t *testing.T, m WorldSetupModel, name string) WorldSetupModel {
	t.Helper()
	m.projectNameInput.SetValue(name)
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, model)
	mm, ok := model.(WorldSetupModel)
	require.True(t, ok, "expected WorldSetupModel, got %T", model)
	_ = cmd
	return mm
}

func TestTargetDirectory_NonCanonicalDirCorrected(t *testing.T) {
	t.Run("single-segment path", func(t *testing.T) {
		m := NewWorldSetupModel("My_Game", "DEV", "")
		assert.Equal(t, "My_Game", m.targetDir, "targetDir should hold the raw arg")
		assert.NotEmpty(t, m.nameErr, "nameErr should be set for non-canonical base")

		// User corrects the name to a canonical one.
		updated := enterNameAndComplete(t, m, "my-game")

		assert.Equal(t, "my-game", updated.targetDirectory(),
			"targetDirectory() should use the corrected canonical name")
		assert.Equal(t, "my-game", updated.targetDir,
			"targetDir should be updated to the corrected name")
		assert.Empty(t, updated.nameErr, "nameErr should be cleared on success")
	})

	t.Run("multi-segment path preserves parent", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "My_Game")
		m := NewWorldSetupModel(dir, "DEV", "")
		require.Equal(t, dir, m.targetDir)

		updated := enterNameAndComplete(t, m, "my-game")

		expected := filepath.Join(filepath.Dir(dir), "my-game")
		assert.Equal(t, expected, updated.targetDirectory())
		assert.Equal(t, expected, updated.targetDir)
	})

	t.Run("relative multi-segment path", func(t *testing.T) {
		m := NewWorldSetupModel("dev/My_Game", "DEV", "")

		updated := enterNameAndComplete(t, m, "my-game")

		assert.Equal(t, filepath.Join("dev", "my-game"), updated.targetDirectory())
	})
}

func TestTargetDirectory_CanonicalDirUnchanged(t *testing.T) {
	m := NewWorldSetupModel("my-game", "DEV", "")
	assert.Equal(t, "my-game", m.targetDir)
	assert.Empty(t, m.nameErr, "nameErr should be empty for canonical base")
	// Init skips the name step, so targetDirectory() returns the original dir.
	assert.Equal(t, "my-game", m.targetDirectory())
}

func TestTargetDirectory_NoDirArgUsesInput(t *testing.T) {
	m := NewWorldSetupModel("", "DEV", "")
	assert.Empty(t, m.targetDir)
	assert.True(t, m.projectNameInput.Focused(), "input should be focused when no dir arg")

	updated := enterNameAndComplete(t, m, "my-game")

	assert.Equal(t, "my-game", updated.targetDirectory())
	assert.Empty(t, updated.targetDir, "targetDir should stay empty — displayDir adds ./")
	assert.Equal(t, "./my-game", updated.displayDir())
}

func TestTargetDirectory_NonCanonicalDirNotCorrected(t *testing.T) {
	m := NewWorldSetupModel("My_Game", "DEV", "")
	require.NotEmpty(t, m.nameErr)

	// User presses Enter without correcting — the name is still non-canonical.
	m.projectNameInput.SetValue("Still_Bad")
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, ok := model.(WorldSetupModel)
	require.True(t, ok)

	assert.NotEmpty(t, mm.nameErr, "nameErr should remain for non-canonical input")
	assert.Equal(t, "My_Game", mm.targetDirectory(),
		"targetDirectory() should not change when the name is still invalid")
	assert.Equal(t, "My_Game", mm.targetDir, "targetDir should not change on failed validation")
}

func TestTargetDirectory_NonCanonicalDirCorrectedToDifferentName(t *testing.T) {
	m := NewWorldSetupModel("My_Game", "DEV", "")

	// User types a completely different canonical name.
	updated := enterNameAndComplete(t, m, "totally-different")

	assert.Equal(t, "totally-different", updated.targetDirectory())
	assert.Equal(t, "totally-different", updated.targetDir)
}

func TestTargetDirectory_EmptyInputDefaultsToStarterGame(t *testing.T) {
	m := NewWorldSetupModel("My_Game", "DEV", "")

	// User clears the input and presses Enter — the default "starter-game" is used.
	m.projectNameInput.SetValue("")
	updated := enterNameAndComplete(t, m, "")

	// handleNameInputKey sets the default before validating.
	assert.Equal(t, "starter-game", updated.targetDirectory())
}

func TestDisplayDir_ConsistentAfterCorrection(t *testing.T) {
	t.Run("non-canonical dir corrected", func(t *testing.T) {
		m := NewWorldSetupModel("My_Game", "DEV", "")
		updated := enterNameAndComplete(t, m, "my-game")
		assert.Equal(t, "my-game", updated.displayDir())
	})

	t.Run("multi-segment non-canonical dir corrected", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "My_Game")
		m := NewWorldSetupModel(dir, "DEV", "")
		updated := enterNameAndComplete(t, m, "my-game")
		expected := filepath.Join(filepath.Dir(dir), "my-game")
		assert.Equal(t, expected, updated.displayDir())
	})

	t.Run("canonical dir unchanged", func(t *testing.T) {
		m := NewWorldSetupModel("my-game", "DEV", "")
		assert.Equal(t, "my-game", m.displayDir())
	})

	t.Run("no dir arg uses ./ prefix", func(t *testing.T) {
		m := NewWorldSetupModel("", "DEV", "")
		updated := enterNameAndComplete(t, m, "my-game")
		assert.Equal(t, "./my-game", updated.displayDir())
	})
}

func TestNewWorldSetupModel_FallbackFocusesInputForNonCanonicalDir(t *testing.T) {
	// The model must focus the name input and set nameErr so the user can fix
	// a non-canonical directory base — this is the documented fallback.
	m := NewWorldSetupModel("My_Game", "DEV", "")
	assert.True(t, m.projectNameInput.Focused(),
		"input must be focused for non-canonical dir so the user can correct it")
	assert.NotEmpty(t, m.nameErr)
	assert.False(t, dnslabel.IsCanonical(filepath.Base(m.targetDir)))
}

func TestNewWorldSetupModel_SkipsNameStepForCanonicalDir(t *testing.T) {
	m := NewWorldSetupModel("my-game", "DEV", "")
	assert.False(t, m.projectNameInput.Focused(),
		"input should not be focused when the dir base is canonical")
	assert.Empty(t, m.nameErr)
	assert.True(t, dnslabel.IsCanonical(filepath.Base(m.targetDir)))
}
