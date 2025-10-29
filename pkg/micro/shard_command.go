package micro

import (
	"context"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// commandManager manages command registration, queuing, and routing for the shard.
type commandManager struct {
	*Service                           // Embedded service for handling network requests
	shard    *Shard                    // Reference to the shard
	channels map[string]commandChannel // Map of command names to their respective channels
	buffer   []Command                 // Reusable buffer for collecting commands from all channels
	auth     *commandVerifier          // Command authentication and verification handler
	tel      *telemetry.Telemetry      // Telemetry instance for logging and metrics
}

// newCommandManager creates a new command manager with the specified options.
func newCommandManager(shard *Shard, opt ShardOptions) (commandManager, error) {
	service, err := NewService(opt.Client, opt.Address, opt.Telemetry)
	if err != nil {
		return commandManager{}, eris.Wrap(err, "failed to create service")
	}

	// TODO: better ttl cache that isn't vulnerable to dos hash eviction.
	// src: https://github.com/Argus-Labs/go-ecs/pull/50#discussion_r2178996978
	// For now this just initializes the cache with an absurdly large size.
	const replayCacheSize = 20 * 1024 * 1024 * 1024 // 20gb
	auth := newCommandVerifer(shard, replayCacheSize, opt.Address, opt.Client)

	return commandManager{
		Service:  service,
		shard:    shard,
		channels: make(map[string]commandChannel),
		buffer:   make([]Command, 0),
		auth:     auth,
		tel:      opt.Telemetry,
	}, nil
}

// Has returns true if the command has been registered.
func (c *commandManager) Has(name string) bool {
	_, exists := c.channels[name]
	return exists
}

// Enqueue receives a command from an external source and stores it in the corresponding channel for
// the given command type. The command name is extracted from the command bytes and used to route
// the command to the appropriate channel.
// Returns an error if the command type is not registered or if validation fails.
func (c *commandManager) Enqueue(cmd *iscv1.Command) error {
	// Unmarshal command bytes to get the command name.
	commandRaw := &iscv1.CommandRaw{}
	if err := proto.Unmarshal(cmd.GetCommandBytes(), commandRaw); err != nil {
		return eris.Wrap(err, "failed to unmarshal command bytes")
	}

	name := commandRaw.GetBody().GetName()
	channel, exists := c.channels[name]
	if !exists {
		return eris.Errorf("unregistered command: %s", name)
	}

	return channel.enqueue(cmd, commandRaw)
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
			return NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), 0)
		default:
			// Continue processing.
		}

		command := &iscv1.Command{}
		if err := req.Payload.UnmarshalTo(command); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "failed to parse request payload"), 0)
		}
		if err := protovalidate.Validate(command); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "failed to validate payload"), 0)
		}

		if err := c.auth.VerifyCommand(command); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "failed to verify command"), 0)
		}

		if err := c.Enqueue(command); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "failed to enqueue command"), 0)
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
	enqueue(*iscv1.Command, *iscv1.CommandRaw) error
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
func (c Channel[T]) enqueue(cmd *iscv1.Command, commandRaw *iscv1.CommandRaw) error {
	var zero T

	commandBody := commandRaw.GetBody()
	if commandBody.GetName() != zero.Name() {
		return eris.Errorf("mismatched command name, expected %s, actual %s", zero.Name(), commandRaw.GetBody().GetName())
	}

	jsonBytes, err := commandBody.GetPayload().MarshalJSON()
	if err != nil {
		return eris.Wrap(err, "failed to marshal command payload to json")
	}

	if err := json.Unmarshal(jsonBytes, &zero); err != nil {
		return eris.Wrap(err, "failed to unmarshal to command")
	}

	c <- Command{
		Signature: cmd.GetSignature(),
		AuthInfo: AuthInfo{
			Mode:          cmd.GetAuthInfo().GetMode(),
			SignerAddress: cmd.GetAuthInfo().GetSignerAddress(),
		},
		CommandBytes: cmd.GetCommandBytes(),
		Command: CommandRaw{
			Timestamp: commandRaw.GetTimestamp().AsTime(),
			Salt:      commandRaw.GetSalt(),
			Body: CommandBody{
				Name:    zero.Name(),
				Address: commandBody.GetAddress(),
				Persona: commandBody.GetPersona().GetId(),
				Payload: zero,
			},
		},
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
