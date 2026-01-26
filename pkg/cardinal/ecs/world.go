package ecs

import (
	"reflect"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// World represents the root ECS state.
type World struct {
	state *worldState

	// Systems.
	initDone    bool               // Tracks if init systems have been executed
	initSystems []initSystem       // Initialization systems, run once during the genesis tick
	scheduler   [3]systemScheduler // Systems schedulers (PreTick, Update, PostTick)

	// system events.
	systemEvents systemEventManager // Manages system events
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
			if err := system.fn(); err != nil {
				return eris.Wrapf(err, "init system %s failed", system.name)
			}
		}
		w.initDone = true
		return nil
	}

	defer w.clearBuffers()

	// Run the systems.
	for i := range w.scheduler {
		if err := w.scheduler[i].Run(); err != nil {
			return err
		}
	}

	return nil
}

// CustomTick allows for a custom update function to be run instead of the registered systems.
// This function is for testing and internal use only!
func (w *World) CustomTick(fn func(*worldState)) {
	fn(w.state)
}

// clearBuffers clears the previous tick's buffers.
func (w *World) clearBuffers() {
	w.systemEvents.clear()
}

// -------------------------------------------------------------------------------------------------
// Serialization methods
// -------------------------------------------------------------------------------------------------

// Serialize converts the World's state to a byte slice for serialization.
// Only serializes the WorldState as components, systems, and managers are recreated on startup.
func (w *World) Serialize() ([]byte, error) {
	snapshot, err := w.state.toProto()
	if err != nil {
		return nil, err
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

// Deserialize populates the World's state from a byte slice.
// This should only be called after the World has been properly initialized with components registered.
func (w *World) Deserialize(data []byte) error {
	var snapshot cardinalv1.CardinalSnapshot
	if err := proto.Unmarshal(data, &snapshot); err != nil {
		return eris.Wrap(err, "failed to unmarshal snapshot")
	}
	if err := w.state.fromProto(&snapshot); err != nil {
		return err
	}
	// Mark init as done to prevent re-running init systems after restore.
	w.initDone = true
	return nil
}

// -------------------------------------------------------------------------------------------------
// Introspection methods
// -------------------------------------------------------------------------------------------------

// CommandTypes returns a map of command names to their reflect.Type.
func (w *World) CommandTypes() map[string]reflect.Type {
	return nil
}

// EventTypes returns a map of event names to their reflect.Type.
func (w *World) EventTypes() map[string]reflect.Type {
	return nil
}

// ComponentTypes returns a map of component names to their reflect.Type.
func (w *World) ComponentTypes() map[string]reflect.Type {
	return w.state.components.types
}
