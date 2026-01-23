package command

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
)

// Command represents a command from a player or external system.
type Command struct {
	Name    string                // The command name
	Address *micro.ServiceAddress // Service address this command is sent to
	Persona string                // Sender's persona
	Payload CommandPayload        // The command payload itself
}

// The interface all user-defined commands must implement.
type CommandPayload interface {
	Name() string
}

// ID is a unique identifier for a command type, used for bookkeeping and performance boosts.
type ID = uint32

// MaxID is the maximum number of command types that can be registered.
const MaxID = math.MaxUint32 - 1

// InvalidID is a sentinel id for errors or when we have exceeded the maximum command count.
const InvalidID = MaxID + 1

// initialCommandBufferCapacity is the starting capacity of command buffers.
const initialCommandBufferCapacity = 128

// Manager manages command registration, queuing, and routing for the shard.
//
// Thread safety: The queues slice is read-only after all commands are registered (which happens
// before the shard starts accepting requests). Each queue has its own mutex to protect concurrent
// access between enqueue (NATS handler goroutines) and drain (tick loop goroutine).
type Manager struct {
	nextID   ID
	catalog  map[string]ID // Command name -> command ID
	queues   []Queue       // Command ID -> sliceQueue[T]
	commands [][]Command   // Command ID -> commands slice
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
	}
}

// Register registers the command type with the command manager.
func (c *Manager) Register(name string, queue Queue) (ID, error) {
	if name == "" {
		return 0, eris.New("command name cannot be empty")
	}

	// If the command is already registered, return the existing ID.
	if id, exists := c.catalog[name]; exists {
		return id, nil
	}

	if c.nextID > MaxID {
		return 0, eris.New("max number of commands exceeded")
	}

	id := c.nextID
	c.catalog[name] = id
	c.commands = append(c.commands, make([]Command, 0, initialCommandBufferCapacity))
	c.queues = append(c.queues, queue)

	c.nextID++
	assert.That(int(c.nextID) == len(c.commands), "command id doesn't match number of commands")

	return id, nil
}

// Enqueue receives a command from an external source and stores it in the corresponding channel for
// the given command type. The command name is extracted from the command and used to route
// the command to the appropriate channel.
// Returns an error if the command type is not registered or if validation fails.
func (c *Manager) Enqueue(command *iscv1.Command) error {
	name := command.GetName()
	id, exists := c.catalog[name]
	if !exists {
		return eris.Errorf("unregistered command: %s", name)
	}
	return c.queues[id].enqueue(command)
}

func (c *Manager) Get(id ID) ([]Command, error) {
	if id >= c.nextID {
		return nil, eris.Errorf("unregistered command id: %d", id)
	}
	return c.commands[id], nil
}

func (c *Manager) Drain() []Command {
	// Clear buffers from previous tick to reuse the slices.
	for id := range c.commands {
		c.commands[id] = c.commands[id][:0]
	}

	all := make([]Command, 0, len(c.commands)*initialCommandBufferCapacity)
	for id, queue := range c.queues {
		queue.drain(&c.commands[id])
		all = append(all, c.commands[id]...)
	}
	return all
}
