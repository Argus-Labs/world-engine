package ecs

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
)

// Event is an interface that all events must implement.
// Events are packets of information that are sent from systems to the outside world.
type Event = Command

// EventKind is a type that represents the kind of event.
type EventKind uint8

const (
	// EventKindDefault is the default event kind.
	EventKindDefault EventKind = 1

	// Reserve 0 for zero value / unspecified event kind in protobuf.
	// Reserve 14 more values (2...15) for future ecs event kind.
	// Users of the `ecs` package should start with CustomEventKindStart for their custom event kinds.
	// Example:
	//
	//	const (
	//    EventKindCustom = iota + ecs.CustomEventKindStart
	//  )
)

const CustomEventKindStart = 16

// RawEvent is the format of ECS output. It has a kind and a payload. The kind determines the type
// of event contained in the payload. Users of ECS can define custom event kinds and handle them in
// their own event handlers.
type RawEvent struct {
	Kind    EventKind // The kind of event
	Payload any       // The payload of the event
}

// eventManager manages the registration and storage of events.
type eventManager struct {
	events   chan RawEvent     // Channel for collecting events emitted by systems
	buffer   []RawEvent        // Buffer for storing events to be outputted
	registry map[string]uint32 // Map from event name to event ID
	nextID   uint32            // Next available event ID
}

// newEventManager creates a new eventManager.
func newEventManager() eventManager {
	const eventChannelCapacity = 1024
	const initialEventBufferCapacity = 128
	return eventManager{
		events:   make(chan RawEvent, eventChannelCapacity),
		buffer:   make([]RawEvent, 0, initialEventBufferCapacity),
		registry: make(map[string]uint32),
		nextID:   0,
	}
}

// register registers an event type and returns its ID. If already registered, returns existing ID.
func (e *eventManager) register(name string) (uint32, error) {
	if name == "" {
		return 0, eris.New("event name cannot be empty")
	}

	if id, exists := e.registry[name]; exists {
		return id, nil
	}

	if e.nextID > MaxCommandID {
		return 0, eris.New("max number of events exceeded")
	}

	e.registry[name] = e.nextID
	e.nextID++
	return e.nextID - 1, nil
}

// enqueue enqueues an event into the eventManager.
func (e *eventManager) enqueue(kind EventKind, payload any) {
	e.events <- RawEvent{Kind: kind, Payload: payload}
}

// getEvents retrieves all events from the eventManager.
func (e *eventManager) getEvents() []RawEvent {
	n := len(e.events)
	for range n {
		select {
		case event := <-e.events:
			e.buffer = append(e.buffer, event)
		default:
		}
	}
	return e.buffer
}

// clear clears the event buffer.
func (e *eventManager) clear() {
	e.buffer = e.buffer[:0]
	assert.That(len(e.buffer) == 0, "event buffer not cleared properly")
}
