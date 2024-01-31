package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/systems"
)

func RegisterSystems(w *World, sys ...systems.System) error {
	return w.systemManager.RegisterSystems(sys...)
}

// Init Registers a system that only runs once on a new game before tick 0.
// TODO(scott): this should probably just be RegisterInitSystems and it should be a function instead of method
func (w *World) Init(system systems.System) {
	w.systemManager.RegisterInitSystem(system)
}

func (w *World) GetSystemNames() []string {
	return w.systemManager.GetSystemNames()
}
