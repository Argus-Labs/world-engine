package textinput

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_SubmitsDefaultOnEnter(t *testing.T) {
	t.Parallel()

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "world-cli"
	ti.SetValue("")
	m := model{input: ti, header: "Enter project name:", hint: "(esc to quit)"}

	// Press enter immediately; should commit default (placeholder)
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := ret.(model)
	require.True(t, mm.finalized)
	assert.False(t, mm.aborted)
	assert.Equal(t, "world-cli", mm.finalValue)

	view := mm.View()
	assert.Contains(t, view, "Enter project name:")
	assert.Contains(t, view, "world-cli")
}

func TestModel_SubmitsTypedValue(t *testing.T) {
	t.Parallel()

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "default"
	ti.SetValue("")
	ti.Focus()
	m := model{input: ti, header: "Enter value:", hint: "(esc to quit)"}

	// Type "abc" then press enter
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = ret.(model)
	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = ret.(model)
	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = ret.(model)

	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := ret.(model)
	require.True(t, mm.finalized)
	assert.Equal(t, "abc", mm.finalValue)
}

func TestModel_AbortWithEsc(t *testing.T) {
	t.Parallel()
	ti := textinput.New()
	m := model{input: ti, header: "Enter:", hint: "(esc to quit)"}

	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := ret.(model)
	assert.True(t, mm.aborted)
}

func TestModel_ViewShowsPromptAndHint(t *testing.T) {
	t.Parallel()
	ti := textinput.New()
	ti.Prompt = ""
	m := model{input: ti, header: "Enter project name:", hint: "(esc to quit)"}

	view := m.View()
	assert.Contains(t, view, "Enter project name:")
	assert.Contains(t, view, "(esc to quit)")
}

func TestModel_ViewWithURLDoesNotTruncateScheme(t *testing.T) {
	t.Parallel()

	ti := textinput.New()
	ti.Prompt = ""
	m := model{input: ti, header: "Hit enter to open https://login.argus.gg/ in your browser:", hint: "(esc to quit)"}

	view := m.View()
	assert.Contains(t, view, "Hit enter to open https://login.argus.gg/ in your browser:")
}
