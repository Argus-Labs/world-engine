package ecs

import (
	"time"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

// World represents the root ECS state.
type World struct {
	state               *worldState
	initDone            bool                  // Tracks if init systems have been executed
	initSystems         []initSystem          // Initialization systems, run once during the genesis tick
	scheduler           [3]systemScheduler    // Systems schedulers (PreTick, Update, PostTick)
	systemEvents        systemEventManager    // Manages system events
	onComponentRegister func(Component) error // Callback called when a component is registered
}

// NewWorld creates a new World instance.
func NewWorld() *World {
	world := &World{
		state:        newWorldState(),
		initDone:     false,
		initSystems:  make([]initSystem, 0),
		scheduler:    [3]systemScheduler{},
		systemEvents: newSystemEventManager(),
	}

	systemHookNames := [3]SystemHook{PreUpdate, Update, PostUpdate}
	for i := range world.scheduler {
		world.scheduler[i] = newSystemScheduler()
		world.scheduler[i].systemHook = systemHookNames[i]
	}

	return world
}

// Init initializes the system schedulers by creating their schedules.
func (w *World) Init() {
	for i := range w.scheduler {
		w.scheduler[i].createSchedule()
	}
}

// Tick passes external events into the event manager and executes the
// registered systems in order. If any system returns an error, the entire tick is considered
// failed, changes are discarded, and the error is returned. If the tick succeeds, the events
// emmitted during the tick is returned.
func (w *World) Tick() error {
	// Run init systems once on first tick.
	if !w.initDone {
		for _, system := range w.initSystems {
			system.fn()
		}
		w.initDone = true
		return nil
	}

	// Clear system events after each tick.
	defer w.systemEvents.clear()

	// Run the systems.
	for i := range w.scheduler {
		w.scheduler[i].Run()
	}

	return nil
}

// SetOnSystemRun sets a callback invoked after each system execution.
// Must be called before Init.
func (w *World) OnSystemRun(fn func(name string, systemHook SystemHook, startTime, endTime time.Time)) {
	for i := range w.scheduler {
		w.scheduler[i].onSystemRun = fn
	}
}

// Schedules returns the dependency graphs for all execution phases.
func (w *World) Schedules() []ScheduleInfo {
	schedules := make([]ScheduleInfo, len(w.scheduler))
	for i := range w.scheduler {
		schedules[i] = w.scheduler[i].scheduleInfo()
	}
	return schedules
}

func (w *World) OnComponentRegister(callback func(zero Component) error) {
	w.onComponentRegister = callback
}

// -------------------------------------------------------------------------------------------------
// Serialization methods
// -------------------------------------------------------------------------------------------------

// ToProto converts the World's state to a proto message.
// Only serializes the WorldState as components, systems, and managers are recreated on startup.
func (w *World) ToProto() (*cardinalv1.WorldState, error) {
	return w.state.toProto()
}

// FromProto populates the World's state from a proto message.
// This should only be called after the World has been properly initialized with components registered.
func (w *World) FromProto(pb *cardinalv1.WorldState) error {
	if err := w.state.fromProto(pb); err != nil {
		return err
	}
	// Mark init as done to prevent re-running init systems after restore.
	w.initDone = true
	return nil
}

// Reset clears the world state back to its initial empty state.
// Components remain registered but all entities and archetypes are cleared.
func (w *World) Reset() {
	w.state.reset()
	w.initDone = false
}
