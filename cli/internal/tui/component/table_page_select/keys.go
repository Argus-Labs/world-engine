package tablepageselect

import "github.com/charmbracelet/bubbles/key"

// ExtraKey binds a shortcut to the action returned by the picker.
type ExtraKey struct {
	Binding key.Binding
	Action  string
	// Secondary hides the binding until the legend expands.
	Secondary bool
}

type pickerLegendKeys struct {
	Nav    key.Binding
	Select key.Binding
	Quit   key.Binding
}

func defaultPickerLegendKeys() pickerLegendKeys {
	return pickerLegendKeys{
		Nav:    key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓ or num", "nav")),
		Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Quit:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "quit")),
	}
}
