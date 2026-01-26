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

// MaxID is the maximum number of event types that can be registered.
const maxID = math.MaxUint32 - 1

// Manager manages event registration, stores events emitted by systems, and dispatches their
// handlers at the end of every tick. Event IDs are only used to check for duplicate WithEvent
// fields in a system state.
type Manager struct {
	nextID   uint32            // Next available event ID
	catalog  map[string]uint32 // Event name -> event ID
	handlers []Handler         // Event handlers, indexed by event kind
	channel  chan Event        // Channel for collecting events emitted by systems
	buffer   []Event           // Overflow buffer for when channel is full
	mu       sync.Mutex        // Mutex for buffer access during flush
}

// NewManager creates a new event manager.
func NewManager() Manager {
	return Manager{
		nextID:   0,
		catalog:  make(map[string]uint32),
		handlers: make([]Handler, 0, math.MaxUint8),
		channel:  make(chan Event, defaultEventChannelCapacity),
		buffer:   make([]Event, 0, initialEventBufferCapacity),
	}
}

// Register registers the event type with the event manager.
func (m *Manager) Register(name string, kind Kind) (uint32, error) {
	if name == "" {
		return 0, eris.New("event name cannot be empty")
	}

	// If the command is already registered, return the existing ID.
	if id, exists := m.catalog[name]; exists {
		return id, nil
	}

	if m.nextID > maxID {
		return 0, eris.New("max number of events exceeded")
	}

	id := m.nextID
	m.catalog[name] = id
	m.nextID++
	return id, nil
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

	if len(errs) > 0 {
		return eris.Errorf("event dispatch encountered %d error(s): %v", len(errs), errs)
	}
	return nil
}

// RegisterHandler registers the handler function for a specific event kind.
func (m *Manager) RegisterHandler(kind Kind, fn Handler) {
	m.handlers[kind] = fn
}
