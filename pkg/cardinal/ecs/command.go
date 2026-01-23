package ecs

import (
	"math"
	"reflect"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/command"
	"github.com/rotisserie/eris"
)

// CommandID is a unique identifier for a command type.
type CommandID uint32

// MaxCommandID is the maximum number of command types that can be registered.
const MaxCommandID = math.MaxUint32 - 1

// Command is the interface that all commands must implement.
// Commands are predefined user actions that are handled by systems.
type Command interface { //nolint:iface // ecs.Command must be a subset of micro.ShardCommand
	command.CommandPayload
}

// commandManager manages the registration and storage of commands.
type commandManager struct {
	nextID   CommandID               // The next command ID
	catalog  map[string]CommandID    // Command name -> Command ID
	commands [][]command.Command     // Command ID -> command
	types    map[string]reflect.Type // Command name -> reflect.Type
}

// newCommandManager creates a new commandManager.
func newCommandManager() commandManager {
	return commandManager{
		nextID:   0,
		catalog:  make(map[string]CommandID),
		commands: make([][]command.Command, 0),
		types:    make(map[string]reflect.Type),
	}
}

// register registers a new command type. If the command is already registered, the existing ID
// is returned.
func (c *commandManager) register(name string, typ reflect.Type) (CommandID, error) {
	if name == "" {
		return 0, eris.New("command name cannot be empty")
	}

	// If the command is already registered, return the existing ID.
	if id, exists := c.catalog[name]; exists {
		return id, nil
	}

	if c.nextID > MaxCommandID {
		return 0, eris.New("max number of commands exceeded")
	}

	const initialCommandBufferCapacity = 128
	c.catalog[name] = c.nextID
	c.commands = append(c.commands, make([]command.Command, 0, initialCommandBufferCapacity))
	c.types[name] = typ
	c.nextID++
	assert.That(int(c.nextID) == len(c.commands), "command id doesn't match number of commands")

	return c.nextID - 1, nil
}

// get retrieves a list of commands for a given command name.
func (c *commandManager) get(name string) ([]command.Command, error) {
	id, exists := c.catalog[name]
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
// All commands are assumed to be pre-validated by the micro layer (command.Manager.Enqueue),
// which rejects unregistered commands before they reach ECS. An unknown command name here indicates
// a mismatch between micro and ECS command registration, which is a programming error, so we should
// fail fast (and loudly) instead of silently ignoring it.
func (c *commandManager) receiveCommands(commands []command.Command) {
	for _, command := range commands {
		id, exists := c.catalog[command.Name]
		assert.That(exists, "command %s is not registered", command.Name)
		c.commands[id] = append(c.commands[id], command)
	}
}
