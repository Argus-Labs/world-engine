// Package keys holds bindings and help rendering shared by TUI components.
package keys

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Legend renders primary bindings and an optional secondary row.
type Legend struct {
	help help.Model
}

// NewLegend returns a collapsed legend.
func NewLegend() Legend {
	return Legend{help: help.New()}
}

// SetWidth prevents the legend from wrapping beyond its allocated row.
func (l *Legend) SetWidth(width int) {
	l.help.Width = width
}

// HandleToggle expands or collapses the legend when the help key is pressed.
func (l *Legend) HandleToggle(msg tea.KeyMsg, hasSecondary bool) bool {
	if !key.Matches(msg, l.toggleBinding(hasSecondary)) {
		return false
	}
	l.help.ShowAll = !l.help.ShowAll
	return true
}

func (l *Legend) toggleBinding(hasSecondary bool) key.Binding {
	description := "more"
	if l.help.ShowAll {
		description = "less"
	}
	binding := key.NewBinding(
		key.WithKeys("?", "/"),
		key.WithHelp("?", description),
	)
	binding.SetEnabled(hasSecondary)
	return binding
}

// View renders primary bindings and, when expanded, secondary bindings.
func (l *Legend) View(primary, secondary []key.Binding) string {
	primary = append(primary, l.toggleBinding(len(secondary) > 0))
	view := l.help.ShortHelpView(primary)
	if l.help.ShowAll && len(secondary) > 0 {
		view += "\n" + l.help.ShortHelpView(secondary)
	}
	return view
}
