package ecs

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
)

// systemEventID is a unique identifier for a system event type.
// It is used internally to track and manage system event types efficiently.
type systemEventID = uint32

// maxSystemEventID is the maximum number of system event types that can be registered.
const maxSystemEventID = math.MaxUint32 - 1

// SystemEvent is an interface that all system events must implement.
// SystemEvents are events emitted by a system to be handled by another system.
type SystemEvent = Command

// systemEventManager manages the registration and storage of system events.
type systemEventManager struct {
	nextID   systemEventID            // The next system event ID
	registry map[string]systemEventID // System event name -> System event ID
	events   [][]any                  // System event ID -> System event
}

// newSystemEventManager creates a new systemEventManager.
func newSystemEventManager() systemEventManager {
	return systemEventManager{
		nextID:   0,
		registry: make(map[string]systemEventID),
		events:   make([][]any, 0),
	}
}

// register registers a new system event type. If the system event is already registered, the
// existing id is returned.
func (s *systemEventManager) register(name string) (systemEventID, error) {
	if name == "" {
		return 0, eris.New("system event name cannot be empty")
	}

	if id, exists := s.registry[name]; exists {
		return id, nil
	}

	if s.nextID > maxSystemEventID {
		return 0, eris.New("max number of system events exceeded")
	}

	const initialEventBufferCapacity = 128
	s.registry[name] = s.nextID
	s.events = append(s.events, make([]any, 0, initialEventBufferCapacity))
	s.nextID++
	assert.That(int(s.nextID) == len(s.events), "system event id doesn't match number of system events")

	return s.nextID - 1, nil
}

// get retrieves a list of system events for a given system event name.
func (s *systemEventManager) get(name string) ([]any, error) {
	id, exists := s.registry[name]
	if !exists {
		return nil, eris.Errorf("system event %s not registered", name)
	}
	return s.events[id], nil
}

// enqueue enqueues a system event to be handled by another system. This function is not safe for
// concurrent use. It expects the scheduler to correctly order the systems so that there are no
// concurrent access to the slices.
func (s *systemEventManager) enqueue(name string, systemEvent SystemEvent) error {
	id, exists := s.registry[name]
	if !exists {
		return eris.Errorf("system event %s not registered", name)
	}
	s.events[id] = append(s.events[id], systemEvent)
	return nil
}

// clear clears the system event buffer.
func (s *systemEventManager) clear() {
	for id := range s.events {
		s.events[id] = s.events[id][:0]
		assert.That(len(s.events[id]) == 0, "system events not cleared properly")
	}
}
