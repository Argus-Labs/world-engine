package logtail

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
)

// ReloadKeys contains the reload bindings shared by the picker and tail view.
type ReloadKeys struct {
	Reload      key.Binding
	PurgeReload key.Binding
}

// DefaultReloadKeys returns the default reload bindings.
func DefaultReloadKeys() ReloadKeys {
	return ReloadKeys{
		Reload: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "reload"),
		),
		PurgeReload: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "purge & reload"),
		),
	}
}

// WithEnabled sets whether both bindings match input and appear in help.
func (k ReloadKeys) WithEnabled(enabled bool) ReloadKeys {
	k.Reload.SetEnabled(enabled)
	k.PurgeReload.SetEnabled(enabled)
	return k
}

// ExtraAction is a caller-supplied action that runs without closing the tail view.
type ExtraAction struct {
	Binding key.Binding
	Run     func(context.Context) (string, error)
}

func secondaryBindings(actions []ExtraAction) []key.Binding {
	bindings := make([]key.Binding, 0, len(actions))
	for _, action := range actions {
		bindings = append(bindings, action.Binding)
	}
	return bindings
}
