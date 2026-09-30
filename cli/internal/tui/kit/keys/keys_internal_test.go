package keys

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func bind(k, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc))
}

func TestLegend_ViewLines(t *testing.T) {
	t.Parallel()
	legend := NewLegend()
	primary := []key.Binding{bind("a", "alpha")}
	secondary := []key.Binding{bind("b", "beta")}

	collapsed := plain(legend.View(primary, secondary))
	assert.Contains(t, collapsed, "a alpha")
	assert.NotContains(t, collapsed, "beta")
	assert.Contains(t, collapsed, "? more")

	assert.True(t, legend.HandleToggle(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}, true))
	expanded := plain(legend.View(primary, secondary))
	assert.Contains(t, expanded, "a alpha")
	assert.Contains(t, expanded, "b beta")
	assert.Contains(t, expanded, "? less")
	assert.Equal(t, 2, len(splitLines(expanded)))

	assert.True(t, legend.HandleToggle(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")}, true))
	assert.Contains(t, plain(legend.View(primary, secondary)), "? more")
}

func TestLegend_HandleToggleDisabledWithoutSecondaryBindings(t *testing.T) {
	t.Parallel()
	legend := NewLegend()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")}
	assert.False(t, legend.HandleToggle(msg, false))
	assert.NotContains(t, plain(legend.View([]key.Binding{bind("a", "alpha")}, nil)), "more")
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

// plain keeps assertions independent of terminal color settings.
func plain(s string) string { return ansi.Strip(s) }
