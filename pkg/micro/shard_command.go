package micro

import (
	"context"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"
)

// commandManager manages command registration, queuing, and routing for the shard.
type commandManager struct {
	*Service                           // Embedded service for handling network requests
	shard    *Shard                    // Reference to the shard
	address  *ServiceAddress           // This shard's address for command validation
	channels map[string]commandChannel // Map of command names to their respective channels
	buffer   []Command                 // Reusable buffer for collecting commands from all channels
	tel      *telemetry.Telemetry      // Telemetry instance for logging and metrics
}

// newCommandManager creates a new command manager with the specified options.
func newCommandManager(shard *Shard, opt ShardOptions) (commandManager, error) {
	service, err := NewService(opt.Client, opt.Address, opt.Telemetry)
	if err != nil {
		return commandManager{}, eris.Wrap(err, "failed to create service")
	}

	return commandManager{
		Service:  service,
		shard:    shard,
		address:  opt.Address,
		channels: make(map[string]commandChannel),
		buffer:   make([]Command, 0),
		tel:      opt.Telemetry,
	}, nil
}

// Has returns true if the command has been registered.
func (c *commandManager) Has(name string) bool {
	_, exists := c.channels[name]
	return exists
}

// Enqueue receives a command from an external source and stores it in the corresponding channel for
// the given command type. The command name is extracted from the command and used to route
// the command to the appropriate channel.
// Returns an error if the command type is not registered or if validation fails.
func (c *commandManager) Enqueue(command *iscv1.Command) error {
	if err := c.validateCommand(command); err != nil {
		return eris.Wrap(err, "command validation failed")
	}

	name := command.GetName()
	channel, exists := c.channels[name]
	if !exists {
		return eris.Errorf("unregistered command: %s", name)
	}

	return channel.enqueue(command)
}

// validateCommand validates a command's structure and destination address.
func (c *commandManager) validateCommand(command *iscv1.Command) error {
	if err := protovalidate.Validate(command); err != nil {
		return eris.Wrap(err, "failed to validate command")
	}

	if String(c.address) != String(command.GetAddress()) {
		return eris.New("command address doesn't match shard address")
	}

	return nil
}

// GetTickData retrieves all pending commands from all registered channels and returns them as a
// slice. It reuses an internal buffer for efficiency and drains all channels completely.
// The returned slice is valid until the next call to GetCommands.
func (c *commandManager) GetTickData() TickData {
	// Clear buffers from previous tick to reuse the slices.
	c.buffer = c.buffer[:0]

	for _, ch := range c.channels {
		n := ch.length()
		for range n {
			c.buffer = append(c.buffer, ch.dequeue())
		}
	}

	return TickData{Commands: c.buffer}
}

// registerCommand registers a command type with the manager and creates network endpoints for leaders.
func registerCommand[T ShardCommand](c *commandManager) error {
	var zero T
	name := zero.Name()

	// If command is already registered, return early (idempotent).
	if _, exists := c.channels[name]; exists {
		return nil
	}

	c.channels[name] = newChannel[T]()

	if c.shard.Mode() != ModeLeader {
		return nil
	}

	// Only leader nodes have to register the command handler endpoints.
	return c.AddGroup("command").AddEndpoint(name, func(ctx context.Context, req *Request) *Response {
		// Check if shard is shutting down.
		select {
		case <-ctx.Done():
			return NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), codes.Canceled)
		default:
			// Continue processing.
		}

		command := &iscv1.Command{}
		if err := req.Payload.UnmarshalTo(command); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "failed to parse request payload"), codes.InvalidArgument)
		}

		if err := c.Enqueue(command); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "failed to enqueue command"), codes.InvalidArgument)
		}

		return NewSuccessResponse(req, nil)
	})
}

// -------------------------------------------------------------------------------------------------
// Generic command channel
// -------------------------------------------------------------------------------------------------

// commandChannel defines the interface for command queuing operations.
// It provides methods to enqueue commands, dequeue them, and check the queue length.
type commandChannel interface {
	enqueue(*iscv1.Command) error
	dequeue() Command
	length() int
}

var _ commandChannel = newChannel[ShardCommand]()

// Channel is a generic buffered channel for handling commands of a specific type.
// It implements the commandChannel interface and provides type-safe command processing.
type Channel[T ShardCommand] chan Command

// newChannel creates a new buffered command channel with a default buffer size.
func newChannel[T ShardCommand]() Channel[T] {
	const defaultChannelBufferSize = 1024
	return make(chan Command, defaultChannelBufferSize)
}

// enqueue validates and adds a command to the channel. It performs type checking to ensure the
// command matches the expected type T, unmarshals the command payload, and sends it to the channel.
// Returns an error if validation fails or marshaling/unmarshaling operations fail.
func (c Channel[T]) enqueue(command *iscv1.Command) error {
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
func (c Channel[T]) dequeue() Command {
	return <-c
}

// length returns the current number of commands waiting in the channel buffer.
func (c Channel[T]) length() int {
	return len(c)
}
