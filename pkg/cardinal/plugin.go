package cardinal

// Plugin defines a self-contained extension that can register systems, components,
// commands, and events with a World. Plugins allow reusable game logic to be packaged
// and shared across projects.
//
// Components, commands, and events are automatically registered when referenced by system
// state fields (via Ref[T], WithCommand[T], WithEvent[T]), so a plugin's Register method
// typically only needs to call RegisterSystem.
//
// Example:
//
//	type MyPlugin struct{ config MyConfig }
//
//	func (p *MyPlugin) Register(world *cardinal.World) error {
//	    cardinal.RegisterSystem(world, MyInitSystem, cardinal.WithHook(cardinal.Init))
//	    cardinal.RegisterSystem(world, MyTickSystem)
//	    return nil
//	}
type Plugin interface {
	Register(world *World)
}

// RegisterPlugin registers a plugin with the world. Must be called before StartGame().
// Panics if the plugin fails to register, consistent with other registration functions.
func RegisterPlugin(world *World, plugin Plugin) {
	plugin.Register(world)
}
