package ecs

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/rotisserie/eris"
)

// CommandID is a unique identifier for a command type.
type CommandID uint32

// MaxCommandID is the maximum number of command types that can be registered.
const MaxCommandID = math.MaxUint32 - 1

// Command is the interface that all commands must implement.
// Commands are predefined user actions that are handled by systems.
type Command interface { //nolint:iface // ecs.Command must be a subset of micro.ShardCommand
	micro.ShardCommand
}

// commandManager manages the registration and storage of commands.
type commandManager struct {
	nextID   CommandID            // The next command ID
	registry map[string]CommandID // Command name -> Command ID
	commands [][]micro.Command    // Command ID -> command
}

// newCommandManager creates a new commandManager.
func newCommandManager() commandManager {
	return commandManager{
		nextID:   0,
		registry: make(map[string]CommandID),
		commands: make([][]micro.Command, 0),
	}
}

// register registers a new command type. If the command is already registered, the existing ID
// is returned.
func (c *commandManager) register(name string) (CommandID, error) {
	if name == "" {
		return 0, eris.New("command name cannot be empty")
	}

	// If the command is already registered, return the existing ID.
	if id, exists := c.registry[name]; exists {
		return id, nil
	}

	if c.nextID > MaxCommandID {
		return 0, eris.New("max number of commands exceeded")
	}

	const initialCommandBufferCapacity = 128
	c.registry[name] = c.nextID
	c.commands = append(c.commands, make([]micro.Command, 0, initialCommandBufferCapacity))
	c.nextID++
	assert.That(int(c.nextID) == len(c.commands), "command id doesn't match number of commands")

	return c.nextID - 1, nil
}

// get retrieves a list of commands for a given command name.
func (c *commandManager) get(name string) ([]micro.Command, error) {
	id, exists := c.registry[name]
	if !exists {
		return nil, eris.Errorf("command %s is not registered", name)
	}
	return c.commands[id], nil
}

// clear clears the command buffer.
func (c *commandManager) clear() {
	for id := range c.commands {
		c.commands[id] = c.commands[id][:0]
		assert.That(len(c.commands[id]) == 0, "commands not cleared properly")
	}
}

// receiveCommands receives a list of commands and stores them in the commandManager.
func (c *commandManager) receiveCommands(commands []micro.Command) error {
	for _, command := range commands {
		id, exists := c.registry[command.Command.Body.Name]
		if !exists {
			return eris.Errorf("command %s is not registered", command.Command.Body.Name)
		}
		c.commands[id] = append(c.commands[id], command)
	}
	return nil
}
