package ecs

import "pkg.world.dev/world-engine/cardinal/ecs/systems"

func (e *Engine) RegisterSystems(systems ...systems.System) error {
	return e.systemManager.RegisterSystems(systems...)
}

func (e *Engine) GetSystemNames() []string {
	return e.systemManager.GetSystemNames()
}

func (e *Engine) AddInitSystem(system systems.System) {
	e.systemManager.RegisterInitSystem(system)
}
