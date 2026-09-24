package command

import (
	"math"
	"sync"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
)

// Command represents a command from a player or external system.
type Command struct {
	Name    string                // The command name
	Address *micro.ServiceAddress // Service address this command is sent to
	Persona string                // Sender's persona
	Payload Payload               // The command payload itself
}

// Payload is the interface all command payloads must implement.
type Payload interface {
	schema.Serializable
}

// ID is a unique identifier for a command type, used for bookkeeping and performance boosts.
type ID = uint32

// MaxID is the maximum number of command types that can be registered.
const MaxID = math.MaxUint32 - 1

// InvalidID is a sentinel id for errors or when we have exceeded the maximum command count.
const InvalidID = MaxID + 1

// ErrStopped is returned by Enqueue after SetStopped has marked the manager as stopped. The run
// loop is the only drainer of the command queue, so once it has exited no accepted command will
// ever be processed; callers should surface this as a "service shutting down" error (and a signal
// to retry) rather than a client-side or validation error.
var ErrStopped = eris.New("command manager is stopped; commands are no longer accepted")

// initialCommandBufferCapacity is the starting capacity of command buffers.
const initialCommandBufferCapacity = 128

// Manager manages command registration and stores commands received to be passed to the ECS world.
// Command IDs are mainly used for quick lookup and to check for duplicate WithCommand fields in
// a system state.
//
// The stopped flag is guarded by stop.mu and is checked atomically with the per-queue enqueue under
// that same lock, so once SetStopped runs no caller can receive a success ack for a command that is
// then silently dropped (and no command can slip in after SetStopped counted the pending ones). The
// guard lives behind a pointer so Manager stays safe to copy as a value, the way NewWorld stores it.
type Manager struct {
	nextID   ID            // Next available command ID
	catalog  map[string]ID // Command name -> command ID
	queues   []Queue       // queue for incoming commands, indexed by command ID
	commands [][]Command   // read-only commands slice used by ECS systems, indexed by command ID
	stop     *stopGuard    // guards the stopped flag and serializes it with Enqueue
}

// stopGuard holds the manager's stopped state. It is heap-allocated by NewManager so that copying a
// Manager value (e.g. into a World field) copies the pointer, never the mutex itself.
type stopGuard struct {
	mu      sync.Mutex
	stopped bool
}

// NewManager creates a new command manager.
func NewManager() Manager {
	// We don't have to preallocate the slices as allocations only happen during command registration,
	// which happens before we start accepting requests and run systems.
	return Manager{
		nextID:   0,
		catalog:  make(map[string]ID),
		queues:   make([]Queue, 0),
		commands: make([][]Command, 0),
		stop:     &stopGuard{},
	}
}

// Register registers the command type with the command manager.
func (m *Manager) Register(name string, queue Queue) (ID, error) {
	if name == "" {
		return 0, eris.New("command name cannot be empty")
	}

	// If the command is already registered, return the existing ID.
	if id, exists := m.catalog[name]; exists {
		return id, nil
	}

	if m.nextID > MaxID {
		return 0, eris.New("max number of commands exceeded")
	}

	id := m.nextID
	m.catalog[name] = id
	m.commands = append(m.commands, make([]Command, 0, initialCommandBufferCapacity))
	m.queues = append(m.queues, queue)

	m.nextID++
	assert.That(int(m.nextID) == len(m.commands), "command id doesn't match number of commands")

	return id, nil
}

// Enqueue stores a command in its corresponding queue. The queues map isn't lock protected, and it
// is expected that there exists only 1 caller for each command type, therefore each caller reads
// a different key. This is ok because concurrent reads on Go maps are allowed.
//
// Enqueue is gated by the manager's stopped flag: once SetStopped has been called (the run loop has
// exited and no tick will ever drain the queue), Enqueue returns ErrStopped and the command is not
// stored. The stopped check and the per-queue enqueue happen under the same lock (stop.mu) so that
// SetStopped cannot drain the queue between the check and the store — without that atomicity a
// caller could receive a success ack for a command that is then silently dropped.
func (m *Manager) Enqueue(command *iscv1.Command) error {
	// Enqueue expects callers to validate the command, so here we just assert for defense in depth.
	// NOTE: one extra assertion that we can't put here is if command.address == this shard.address.
	// The caller must be responsible for checking this.
	assert.That(command.GetName() != "", "command has empty name")
	assert.That(command.GetAddress() != nil, "command has nil address")
	assert.That(command.GetPersona() != nil, "command has nil persona")
	// Payload may be empty: a command whose proto message has no set fields serializes to zero
	// bytes (e.g. lobby_heartbeat). Identity lives in name/address/persona, so an empty payload
	// is valid — only those three are real invariants.

	m.stop.mu.Lock()
	defer m.stop.mu.Unlock()
	if m.stop.stopped {
		return ErrStopped
	}

	// We're doing 2 lookups here to keep the Enqueue caller simple, at the cost of less performance.
	// If this is determined to be a bottleneck in the future, do what callers of Get do and store
	// the ID of the command in the caller, so we can do a direct index with Enqueue(id, command).
	name := command.GetName()
	id, exists := m.catalog[name]
	if !exists {
		return eris.Errorf("unregistered command: %s", name)
	}
	return m.queues[id].Enqueue(command)
}

// Get retrieves a slice of commands given the command ID. The ID is returned from Register, and
// callers are expected to store it for calls to Get. This API is used vs using the command's name
// as the index as that requires an extra map lookup. We sacrifice extra complexity at the caller
// to make sure lookups are fast as Get is a hot path as it is called every tick.
func (m *Manager) Get(id ID) ([]Command, error) {
	if id >= m.nextID {
		return nil, eris.Errorf("unregistered command id: %d", id)
	}
	return m.commands[id], nil
}

// Drain collects commands from the queues to read-only command buffers. It also returns a list of
// all commands collected thus far (used by the transaction log). Drain is expected to be called at
// the start of each tick.
func (m *Manager) Drain() []Command {
	// Clear buffers from previous tick to reuse the slices.
	for id := range m.commands {
		m.commands[id] = m.commands[id][:0]
	}

	all := make([]Command, 0, len(m.commands)*initialCommandBufferCapacity)
	for id, queue := range m.queues {
		queue.Drain(&m.commands[id])
		all = append(all, m.commands[id]...)
	}
	return all
}

// Clear discards all pending commands from both queues and buffers.
func (m *Manager) Clear() {
	for id := range m.queues {
		m.queues[id].Drain(&m.commands[id])
		m.commands[id] = m.commands[id][:0]
	}
}

// SetStopped marks the manager as stopped: subsequent (and in-flight) Enqueue calls return ErrStopped
// instead of storing their commands, since no tick will ever drain the queue after the run loop has
// exited. It returns the number of commands still pending in the queues so the caller can log the
// loss. SetStopped is idempotent; a second call returns 0.
//
// The stopped flag and the per-queue length reads share stop.mu with Enqueue, so under a concurrent
// Enqueue each command either fully precedes SetStopped (it is counted here) or observes stopped and
// is rejected — there is no window in which an accepted command escapes both the count and the
// rejection and is silently dropped.
func (m *Manager) SetStopped() int {
	m.stop.mu.Lock()
	defer m.stop.mu.Unlock()
	if m.stop.stopped {
		return 0
	}
	m.stop.stopped = true
	n := 0
	for _, q := range m.queues {
		n += q.Len()
	}
	return n
}

// -------------------------------------------------------------------------------------------------
// Test helpers
// -------------------------------------------------------------------------------------------------

// Names returns the names of all registered command types.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.catalog))
	for name := range m.catalog {
		names = append(names, name)
	}
	return names
}

// Zero returns a zero-value instance of the named command's payload type.
func (m *Manager) Zero(name string) Payload {
	id, exists := m.catalog[name]
	assert.That(exists, "command doesn't exist")
	return m.queues[id].Zero()
}
