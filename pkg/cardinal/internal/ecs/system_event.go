package ecs

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/rotisserie/eris"
)

// SystemEventID is a unique identifier for a system event type.
// It is used internally to track and manage system event types efficiently.
type SystemEventID = uint32

// maxSystemEventID is the maximum number of system event types that can be registered.
// const maxSystemEventID = math.MaxUint32 - 1

// SystemEvent is an interface that all system events must implement.
// SystemEvents are events emitted by a system to be handled by another system.
type SystemEvent interface { //nolint:iface // may extend later
	schema.Serializable
}

// systemEventManager manages the registration and storage of system events.
type systemEventManager struct {
	nextID  SystemEventID            // The next system event ID
	catalog map[string]SystemEventID // System event name -> System event ID
	events  []abstractSystemEventQueue
}

// newSystemEventManager creates a new systemEventManager.
func newSystemEventManager() systemEventManager {
	return systemEventManager{
		nextID:  0,
		catalog: make(map[string]SystemEventID),
		events:  make([]abstractSystemEventQueue, 0),
	}
}

// register registers a new system event type as abstract SystemEvent. If the system event is
// already registered, the existing id is returned.
func (s *systemEventManager) register(name string, factory systemEventQueueFactory) (SystemEventID, error) {
	if name == "" {
		return 0, eris.New("system event name cannot be empty")
	}

	if seid, exists := s.catalog[name]; exists {
		return seid, nil
	}

	s.catalog[name] = s.nextID
	s.events = append(s.events, factory())
	s.nextID++
	assert.That(int(s.nextID) == len(s.events), "system event id doesn't match number of system events")

	return s.nextID - 1, nil
}

// clear clears the system event buffer.
func (s *systemEventManager) clear() {
	for id := range s.events {
		s.events[id].clear()
		assert.That(s.events[id].len() == 0, "system events not cleared properly")
	}
}

// enqueueSystemEvent enqueues a system event to be handled by another system. The system event must be
// registered before calling this function. This function is not safe for concurrent use. It expects
// the scheduler to correctly order systems so that there are no concurrent access to the slices.
func enqueueSystemEvent[T SystemEvent](s *systemEventManager, systemEvent T) error {
	name := systemEvent.Name()

	seid, exists := s.catalog[name]
	if !exists {
		return eris.Wrapf(ErrSystemEventNotFound, "system event %d", seid)
	}

	queue, ok := s.events[seid].(*systemEventQueue[T])
	assert.That(ok, "unexpected system event type %s", name)

	queue.enqueue(systemEvent)
	return nil
}

// getSystemEvent retrieves a list of system events for a given system event name. The system event must be
// registered before calling this function.
func getSystemEvent[T SystemEvent](s *systemEventManager) ([]T, error) {
	var zero T
	name := zero.Name()

	seid, exists := s.catalog[name]
	if !exists {
		return nil, eris.Wrapf(ErrSystemEventNotFound, "system event %d", seid)
	}

	queue, ok := s.events[seid].(*systemEventQueue[T])
	assert.That(ok, "unexpected system event type %s", name)

	return queue.get(), nil
}

// RegisterSystemEvent registers a component type with the world.
func RegisterSystemEvent[T SystemEvent](world *World) (SystemEventID, error) {
	var zero T
	return world.systemEvents.register(zero.Name(), newSystemEventQueueFactory[T]())
}

// These methods below work on the interface type which creates extra allocations and are only used
// for tests. Prefer the generic methods above.

// enqueueAbstract enqueues a boxed system event by runtime name.
func (s *systemEventManager) enqueueAbstract(systemEvent SystemEvent) error {
	name := systemEvent.Name()

	seid, exists := s.catalog[name]
	if !exists {
		return eris.Wrapf(ErrSystemEventNotFound, "system event %d", seid)
	}

	queue := s.events[seid]
	queue.enqueueAbstract(systemEvent)
	return nil
}

func (s *systemEventManager) getAbstract(name string) ([]SystemEvent, error) {
	seid, exists := s.catalog[name]
	if !exists {
		return nil, eris.Wrapf(ErrSystemEventNotFound, "system event %d", seid)
	}

	queue := s.events[seid]
	return queue.getAbstract(), nil
}

// -------------------------------------------------------------------------------------------------
// System Event Queues
// -------------------------------------------------------------------------------------------------

// systemEventQueueFactory is a function that creates a new abstractSystemEventQueue instance.
type systemEventQueueFactory func() abstractSystemEventQueue

// abstractSystemEventQueue is an internal interface for generic system event queue operations.
type abstractSystemEventQueue interface {
	len() int
	clear()
	enqueueAbstract(SystemEvent)
	getAbstract() []SystemEvent
}

var _ abstractSystemEventQueue = (*systemEventQueue[SystemEvent])(nil)

// systemEventQueue stores system event data of type T.
type systemEventQueue[T SystemEvent] struct {
	events []T
}

// newSystemEventQueue creates a new queue with the specified event type.
func newSystemEventQueue[T SystemEvent]() systemEventQueue[T] {
	const initialEventBufferCapacity = 128
	return systemEventQueue[T]{
		events: make([]T, 0, initialEventBufferCapacity),
	}
}

// newColumnFactory returns a function that constructs a new column of type T.
func newSystemEventQueueFactory[T SystemEvent]() systemEventQueueFactory {
	return func() abstractSystemEventQueue {
		queue := newSystemEventQueue[T]()
		return &queue
	}
}

// len returns the length of the system event slice.
func (s *systemEventQueue[T]) len() int {
	return len(s.events)
}

// clear removes all system events from the queue.
func (s *systemEventQueue[T]) clear() {
	s.events = s.events[:0]
}

// get gets all events in queue order. Whenever possible prefer this method over getAbstract since
// it avoids boxing and per-event type assertions.
func (s *systemEventQueue[T]) get() []T {
	return s.events
}

// getAbstract gets all events in queue order as the abstract SystemEvent type. Use this method only
// when you don't know the concrete type of the system events.
func (s *systemEventQueue[T]) getAbstract() []SystemEvent {
	events := make([]SystemEvent, len(s.events))
	for i, event := range s.events {
		events[i] = event
	}
	return events
}

// enqueue appends a system event to the queue. Whenever possible prefer this method over
// enqueueAbstract since it avoids type assertions and boxing.
func (s *systemEventQueue[T]) enqueue(systemEvent T) {
	s.events = append(s.events, systemEvent)
}

// enqueueAbstract appends a system event to the queue. Use this method only when you don't know
// the concrete type of the system event.
func (s *systemEventQueue[T]) enqueueAbstract(systemEvent SystemEvent) {
	event, ok := systemEvent.(T)
	assert.That(ok, "tried to enqueue wrong system event type")
	s.enqueue(event)
}
