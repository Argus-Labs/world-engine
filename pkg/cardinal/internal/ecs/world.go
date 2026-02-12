package ecs

import (
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
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

	for i := range world.scheduler {
		world.scheduler[i] = newSystemScheduler()
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

func (w *World) OnComponentRegister(callback func(zero Component) error) {
	w.onComponentRegister = callback
}

// -------------------------------------------------------------------------------------------------
// Serialization methods
// -------------------------------------------------------------------------------------------------

// Serialize converts the World's state to a byte slice for serialization.
// Only serializes the WorldState as components, systems, and managers are recreated on startup.
func (w *World) Serialize() ([]byte, error) {
	worldState, err := w.state.toProto()
	if err != nil {
		return nil, err
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(worldState)
}

// Deserialize populates the World's state from a byte slice.
// This should only be called after the World has been properly initialized with components registered.
func (w *World) Deserialize(data []byte) error {
	var worldState cardinalv1.WorldState
	if err := proto.Unmarshal(data, &worldState); err != nil {
		return eris.Wrap(err, "failed to unmarshal world state")
	}
	if err := w.state.fromProto(&worldState); err != nil {
		return err
	}
	// Mark init as done to prevent re-running init systems after restore.
	w.initDone = true
	return nil
}
