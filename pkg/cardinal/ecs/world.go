package ecs

import (
	"github.com/argus-labs/world-engine/pkg/micro"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// World represents the root ECS state with double buffering support.
type World struct {
	state *worldState

	// Systems.
	initSystems []initSystem       // Initialization systems, run once during the genesis tick
	scheduler   [3]systemScheduler // Systems schedulers (PreTick, Update, PostTick)

	// Components, commands, events, system events.
	// components   componentManager   // Component type registry (immutable after world start)
	commands     commandManager     // Receives commands from external sources
	events       eventManager       // Stores events to be emitted to external sources
	systemEvents systemEventManager // Manages system events
}

// NewWorld creates a new World instance.
func NewWorld() *World {
	world := &World{
		state:        newWorldState(),
		initSystems:  make([]initSystem, 0),
		scheduler:    [3]systemScheduler{},
		systemEvents: newSystemEventManager(),
		commands:     newCommandManager(),
		events:       newEventManager(),
		// components:   newComponentManager(),
	}

	for i := range world.scheduler {
		world.scheduler[i] = newSystemScheduler()
	}

	return world
}

// InitSchedulers initializes the system schedulers by creating their schedules.
func (w *World) InitSchedulers() {
	for i := range w.scheduler {
		w.scheduler[i].createSchedule()
	}
}

// InitSystems runs all registered init systems.
func (w *World) InitSystems() error {
	for _, system := range w.initSystems {
		if err := system.fn(); err != nil {
			return eris.Wrapf(err, "init system %s failed", system.name)
		}
	}
	return nil
}

// Tick passes external events into the event manager and executes the
// registered systems in order. If any system returns an error, the entire tick is considered
// failed, changes are discarded, and the error is returned. If the tick succeeds, the events
// emmitted during the tick is returned.
func (w *World) Tick(commands []micro.Command) ([]RawEvent, error) {
	// Receive commands from external sources and clear buffers.
	if err := w.commands.receiveCommands(commands); err != nil {
		return []RawEvent{}, err
	}
	defer w.clearBuffers()

	// Run the systems.
	for i := range w.scheduler {
		if err := w.scheduler[i].Run(); err != nil {
			return []RawEvent{}, err
		}
	}

	// Copy commands and events to the result.
	emittedEvents := w.events.getEvents()
	result := make([]RawEvent, len(emittedEvents))
	copy(result, emittedEvents)

	return result, nil
}

// CustomTick allows for a custom update function to be run instead of the registered systems.
// This function is for testing and internal use only!
func (w *World) CustomTick(fn func(*worldState)) {
	fn(w.state)
}

// clearBuffers clears the previous tick's buffers.
func (w *World) clearBuffers() {
	w.commands.clear()
	w.events.clear()
	w.systemEvents.clear()
}

// -------------------------------------------------------------------------------------------------
// Serialization methods
// -------------------------------------------------------------------------------------------------

// Serialize converts the World's state to a byte slice for serialization.
// Only serializes the WorldState as components, systems, and managers are recreated on startup.
func (w *World) Serialize() ([]byte, error) {
	snapshot, err := w.state.serialize()
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
	return w.state.deserialize(&snapshot)
}
