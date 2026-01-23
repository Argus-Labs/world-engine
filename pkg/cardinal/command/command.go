package command

import (
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/goccy/go-json"
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

// Manager manages command registration, queuing, and routing for the shard.
type Manager struct {
	channels map[string]Channel // Map of command names to their respective channels
	buffer   []Command          // Reusable buffer for collecting commands from all channels
}

// newCommandManager creates a new command manager with the specified options.
func NewManager() Manager {
	return Manager{
		channels: make(map[string]Channel),
		buffer:   make([]Command, 0),
	}
}

func (c *Manager) Register(name string, channel Channel) {
	if _, exists := c.channels[name]; exists {
		return
	}
	c.channels[name] = channel
}

// Has returns true if the command has been registered.
func (c *Manager) Has(name string) bool {
	_, exists := c.channels[name]
	return exists
}

// Enqueue receives a command from an external source and stores it in the corresponding channel for
// the given command type. The command name is extracted from the command and used to route
// the command to the appropriate channel.
// Returns an error if the command type is not registered or if validation fails.
func (c *Manager) Enqueue(command *iscv1.Command) error {
	name := command.GetName()
	channel, exists := c.channels[name]
	if !exists {
		return eris.Errorf("unregistered command: %s", name)
	}

	return channel.enqueue(command)
}

// GetCommands retrieves all pending commands from all registered channels and returns them as a
// slice. It reuses an internal buffer for efficiency and drains all channels completely.
// The returned slice is valid until the next call to GetCommands.
func (c *Manager) GetCommands() []Command {
	// Clear buffers from previous tick to reuse the slices.
	c.buffer = c.buffer[:0]

	for _, ch := range c.channels {
		n := ch.length()
		for range n {
			c.buffer = append(c.buffer, ch.dequeue())
		}
	}

	return c.buffer
}

// -------------------------------------------------------------------------------------------------
// Generic command channel
// -------------------------------------------------------------------------------------------------

// Channel defines the interface for command queuing operations.
// It provides methods to enqueue commands, dequeue them, and check the queue length.
type Channel interface {
	enqueue(*iscv1.Command) error
	dequeue() Command
	length() int
}

var _ Channel = NewChannel[CommandPayload]()

// channel is a generic buffered channel for handling commands of a specific type.
// It implements the commandChannel interface and provides type-safe command processing.
type channel[T CommandPayload] chan Command

// NewChannel creates a new buffered command channel with a default buffer size.
func NewChannel[T CommandPayload]() channel[T] {
	const defaultChannelBufferSize = 1024
	return make(chan Command, defaultChannelBufferSize)
}

// enqueue validates and adds a command to the channel. It performs type checking to ensure the
// command matches the expected type T, unmarshals the command payload, and sends it to the channel.
// Returns an error if validation fails or marshaling/unmarshaling operations fail.
func (c channel[T]) enqueue(command *iscv1.Command) error {
	var zero T

	if command.GetName() != zero.Name() {
		return eris.Errorf("mismatched command name, expected %s, actual %s", zero.Name(), command.GetName())
	}

	jsonBytes, err := command.GetPayload().MarshalJSON()
	if err != nil {
		return eris.Wrap(err, "failed to marshal command payload to json")
	}

	if err := json.Unmarshal(jsonBytes, &zero); err != nil {
		return eris.Wrap(err, "failed to unmarshal to command")
	}

	c <- Command{
		Name:    zero.Name(),
		Address: command.GetAddress(),
		Persona: command.GetPersona().GetId(),
		Payload: zero,
	}
	return nil
}

// dequeue removes and returns the next command from the channel.
// This operation blocks if the channel is empty until a command becomes available.
func (c channel[T]) dequeue() Command {
	return <-c
}

// length returns the current number of commands waiting in the channel buffer.
func (c channel[T]) length() int {
	return len(c)
}
