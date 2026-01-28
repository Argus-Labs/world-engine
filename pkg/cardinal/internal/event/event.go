package event

import (
	"math"
	"sync"

	"github.com/rotisserie/eris"
)

// Event represents an event emitted from a system.
type Event struct {
	Kind    Kind         // The event kind
	Payload EventPayload // The event payload itself
}

// The interface all event payloads must implement.
type EventPayload interface {
	Name() string
}

// Kind is a type that represents the kind of event.
type Kind uint8

const (
	KindDefault           Kind = 0 // The default event type
	KindInterShardCommand Kind = 1 // Inter-shard commands
)

// Handler is a function called to handle emitted events.
type Handler func(Event) error

// TODO: figure out whether to make this configurable.
// defaultEventChannelCapacity is the default size of the event channel.
const defaultEventChannelCapacity = 1024

// initialCommandBufferCapacity is the starting capacity of command buffers.
const initialEventBufferCapacity = 128

// Manager manages event registration, stores events emitted by systems, and dispatches their
// handlers at the end of every tick.
type Manager struct {
	handlers []Handler  // Event handlers, indexed by event kind
	channel  chan Event // Channel for collecting events emitted by systems
	buffer   []Event    // Overflow buffer for when channel is full
	mu       sync.Mutex // Mutex for buffer access during flush
}

// NewManager creates a new event manager.
func NewManager() Manager {
	return Manager{
		handlers: make([]Handler, math.MaxUint8+1),
		channel:  make(chan Event, defaultEventChannelCapacity),
		buffer:   make([]Event, 0, initialEventBufferCapacity),
	}
}

// Enqueue enqueues an event into the eventManager.
// If the channel is full, it flushes the channel to the buffer first.
func (m *Manager) Enqueue(event Event) {
	select {
	case m.channel <- event:
		// Happy path: channel has capacity.
	default:
		// Channel full: flush to buffer, then send.
		m.flush()
		m.channel <- event
	}
}

// flush drains the channel into the buffer. Called when channel is full.
// This method expects the caller to hold tthe mutex lock.
func (m *Manager) flush() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for {
		select {
		case event := <-m.channel:
			m.buffer = append(m.buffer, event)
		default:
			return
		}
	}
}

// Dispatch loops through emitted events and calls their handler functions based on the event kind.
// Returns all errors collected from handlers.
func (m *Manager) Dispatch() error {
	m.flush()

	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for _, event := range m.buffer {
		handler := m.handlers[event.Kind]
		if err := handler(event); err != nil {
			errs = append(errs, err)
		}
	}

	// Clear the buffer after processing.
	m.buffer = m.buffer[:0]

	if len(errs) > 0 {
		return eris.Errorf("event dispatch encountered %d error(s): %v", len(errs), errs)
	}
	return nil
}

// RegisterHandler registers the handler function for a specific event kind.
func (m *Manager) RegisterHandler(kind Kind, fn Handler) {
	m.handlers[kind] = fn
}
