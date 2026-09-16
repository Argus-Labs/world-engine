package cardinal

// Plugin defines a self-contained extension that can register systems, components,
// commands, and events with a World. Plugins allow reusable game logic to be packaged
// and shared across projects.
//
// Commands and events are registered when referenced by system state fields (via
// WithCommand[T] and WithEvent[T]). Components are not: a plugin's Register method must call
// RegisterComponent for every component its systems use, then RegisterSystem.
//
// Example:
//
//	type MyPlugin struct{ config MyConfig }
//
//	func (p *MyPlugin) Register(w *cardinal.World) {
//	    w.RegisterComponent[MyComponent]()
//	    w.RegisterSystem(MyInitSystem, cardinal.WithHook(cardinal.Init))
//	    w.RegisterSystem(MyTickSystem)
//	}
type Plugin interface {
	Register(w *World)
}

// RegisterPlugin registers a plugin with the world. Must be called before StartGame().
// Panics if the plugin fails to register, consistent with other registration functions.
func (w *World) RegisterPlugin(plugin Plugin) {
	plugin.Register(w)
}
