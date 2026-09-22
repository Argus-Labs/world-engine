package cardinal

// Plugin defines a self-contained extension that can register systems, components,
// commands, and events with a World. Plugins allow reusable game logic to be packaged
// and shared across projects.
//
// Nothing registers implicitly: a plugin's Register method must call RegisterComponent,
// RegisterCommand, RegisterEvent, and RegisterSystemEvent for everything its systems use,
// including types that belong to another plugin. Registering a type twice is a no-op.
//
// Example:
//
//	type MyPlugin struct{ config MyConfig }
//
//	func (p *MyPlugin) Register(w *cardinal.World) {
//	    w.RegisterComponent[MyComponent]()
//	    w.RegisterCommand[MyCommand]()
//	    w.RegisterEvent[MyEvent]()
//	    w.RegisterSystem(&MyInitSystem{}, cardinal.WithHook(cardinal.Init))
//	    w.RegisterSystem(&MyTickSystem{})
//	}
type Plugin interface {
	Register(w *World)
}

// RegisterPlugin registers a plugin with the world. Must be called before StartGame().
// Panics if the plugin fails to register, consistent with other registration functions.
func (w *World) RegisterPlugin(plugin Plugin) {
	plugin.Register(w)
}
