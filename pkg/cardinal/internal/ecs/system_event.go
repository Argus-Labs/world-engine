package ecs

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/rotisserie/eris"
)

// SystemEventID is a unique identifier for a system event type.
// It is used internally to track and manage system event types efficiently.
type SystemEventID = uint32

// maxSystemEventID is the maximum number of system event types that can be registered.
const maxSystemEventID = math.MaxUint32 - 1

// SystemEvent is an interface that all system events must implement.
// SystemEvents are events emitted by a system to be handled by another system. Though they are consumed
// in-process, they implement the same Serializable interface as the other wire kinds (generated
// MarshalWire/UnmarshalWire) so they can be consumed uniformly — e.g. streamed as typed values to a
// telemetry/debug service — without a special-case interface.
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

// register registers a typed queue under a system event name. Registering the same type again
// returns its existing ID. Reusing a name for another type returns an error.
func (s *systemEventManager) register[T SystemEvent](name string) (SystemEventID, error) {
	if name == "" {
		return 0, eris.New("system event name cannot be empty")
	}

	if seid, exists := s.catalog[name]; exists {
		if _, ok := s.events[seid].(*systemEventQueue[T]); !ok {
			return 0, eris.Errorf("system event %s already registered with a different type", name)
		}
		return seid, nil
	}

	if s.nextID > maxSystemEventID {
		return 0, eris.New("max number of system events exceeded")
	}

	s.catalog[name] = s.nextID
	queue := newSystemEventQueue[T]()
	s.events = append(s.events, &queue)
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

// enqueue enqueues a system event to be handled by another system. The system event must be
// registered before calling this function. This function is not safe for concurrent use. It expects
// the scheduler to correctly order systems so that there are no concurrent access to the slices.
func (s *systemEventManager) enqueue[T SystemEvent](systemEvent T) error {
	name := systemEvent.Name()

	seid, exists := s.catalog[name]
	if !exists {
		return eris.Wrapf(ErrSystemEventNotFound, "system event %s", name)
	}

	queue, ok := s.events[seid].(*systemEventQueue[T])
	assert.That(ok, "unexpected system event type %s", name)

	queue.enqueue(systemEvent)
	return nil
}

// get retrieves a list of system events for a given system event name. The system event must be
// registered before calling this function.
func (s *systemEventManager) get[T SystemEvent](name string) ([]T, error) {
	seid, exists := s.catalog[name]
	if !exists {
		return nil, eris.Wrapf(ErrSystemEventNotFound, "system event %s", name)
	}

	queue, ok := s.events[seid].(*systemEventQueue[T])
	assert.That(ok, "unexpected system event type %s", name)

	return queue.get(), nil
}

// RegisterSystemEvent registers a system event type with the world.
func (w *World) RegisterSystemEvent[T SystemEvent]() (SystemEventID, error) {
	var zero T
	return w.systemEvents.register[T](zero.Name())
}

// -------------------------------------------------------------------------------------------------
// System Event Queues
// -------------------------------------------------------------------------------------------------

// abstractSystemEventQueue is an internal interface for generic system event queue operations.
type abstractSystemEventQueue interface {
	len() int
	clear()
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

// len returns the length of the system event slice.
func (s *systemEventQueue[T]) len() int {
	return len(s.events)
}

// clear removes all system events from the queue.
func (s *systemEventQueue[T]) clear() {
	s.events = s.events[:0]
}

// get gets all events in queue order.
func (s *systemEventQueue[T]) get() []T {
	return s.events
}

// enqueue appends a system event to the queue.
func (s *systemEventQueue[T]) enqueue(systemEvent T) {
	s.events = append(s.events, systemEvent)
}
