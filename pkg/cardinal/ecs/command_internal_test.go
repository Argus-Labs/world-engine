package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandManager_Register(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command Command
		wantErr bool
	}{
		{
			name:    "successful registration",
			command: AttackPlayerCommand{},
		},
		{
			name:    "empty command name",
			command: InvalidEmptyCommand{},
			wantErr: true,
		},
		{
			name:    "duplicate command name",
			command: AttackPlayerCommand{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cr := newCommandManager()

			// For duplicate test case, register command first
			if tt.name == "duplicate command name" {
				_, err := cr.register(tt.command.Name())
				require.NoError(t, err)
			}

			id, err := cr.register(tt.command.Name())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			storedID, exists := cr.registry[tt.command.Name()]
			assert.True(t, exists)

			assert.Equal(t, id, storedID)
			assert.Len(t, cr.commands, 1)
		})
	}
}

func TestCommandManager_ReceiveCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupFn       func(*testing.T, *commandManager) []string
		inputCommands []micro.Command
		wantErr       bool
		testFn        func(*testing.T, *commandManager, []string)
	}{
		{
			name: "multiple valid commands",
			setupFn: func(t *testing.T, cr *commandManager) []string {
				_, err := cr.register(AttackPlayerCommand{}.Name())
				require.NoError(t, err)

				_, err = cr.register(CreatePlayerCommand{}.Name())
				require.NoError(t, err)

				return []string{AttackPlayerCommand{}.Name(), CreatePlayerCommand{}.Name()}
			},
			inputCommands: []micro.Command{
				{Command: micro.CommandRaw{Body: micro.CommandBody{
					Name: AttackPlayerCommand{}.Name(), Payload: AttackPlayerCommand{}}}},
				{Command: micro.CommandRaw{Body: micro.CommandBody{
					Name: CreatePlayerCommand{}.Name(), Payload: CreatePlayerCommand{}}}},
				{Command: micro.CommandRaw{Body: micro.CommandBody{
					Name: AttackPlayerCommand{}.Name(), Payload: AttackPlayerCommand{}}}},
			},
			testFn: func(t *testing.T, cr *commandManager, commandNames []string) {
				attackPlayerCommands, err := cr.get(AttackPlayerCommand{}.Name())
				require.NoError(t, err)
				assert.Len(t, attackPlayerCommands, 2)

				createPlayerCommands, err := cr.get(CreatePlayerCommand{}.Name())
				require.NoError(t, err)
				assert.Len(t, createPlayerCommands, 1)
			},
		},
		{
			name: "unregistered command name",
			setupFn: func(t *testing.T, cr *commandManager) []string {
				// Don't register the command.
				return []string{AttackPlayerCommand{}.Name()}
			},
			inputCommands: []micro.Command{{Command: micro.CommandRaw{Body: micro.CommandBody{
				Name: AttackPlayerCommand{}.Name(), Payload: AttackPlayerCommand{}}}}},
			wantErr: true,
		},
		{
			name: "empty command list",
			setupFn: func(t *testing.T, cr *commandManager) []string {
				_, err := cr.register(AttackPlayerCommand{}.Name())
				require.NoError(t, err)
				return []string{AttackPlayerCommand{}.Name()}
			},
			inputCommands: []micro.Command{},
			testFn: func(t *testing.T, cr *commandManager, cmdNames []string) {
				// Empty list should process without error
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cr := newCommandManager()

			cmdNames := tt.setupFn(t, &cr)

			err := cr.receiveCommands(tt.inputCommands)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			tt.testFn(t, &cr, cmdNames)
		})
	}
}

func TestCommandManager_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(*testing.T, *commandManager)
		queryName  string
		wantErr    bool
		validateFn func(*testing.T, []micro.Command)
	}{
		{
			name: "get empty commands",
			setupFn: func(t *testing.T, cr *commandManager) {
				_, err := cr.register(AttackPlayerCommand{}.Name())
				require.NoError(t, err)
			},
			queryName: AttackPlayerCommand{}.Name(),
			validateFn: func(t *testing.T, cmds []micro.Command) {
				assert.Empty(t, cmds)
			},
		},
		{
			name: "get unregistered command",
			setupFn: func(t *testing.T, cr *commandManager) {
				_, err := cr.register(AttackPlayerCommand{}.Name())
				require.NoError(t, err)
			},
			queryName: "non-existent-command",
			wantErr:   true,
		},
		{
			name: "get commands successfully",
			setupFn: func(t *testing.T, cr *commandManager) {
				_, err := cr.register(AttackPlayerCommand{}.Name())
				require.NoError(t, err)

				var commands [100]micro.Command
				for i := range 100 {
					commands[i] = micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: AttackPlayerCommand{}.Name(), Payload: AttackPlayerCommand{Value: i}}}}
				}
				err = cr.receiveCommands(commands[:])
				require.NoError(t, err)
			},
			queryName: AttackPlayerCommand{}.Name(),
			validateFn: func(t *testing.T, cmds []micro.Command) {
				assert.Len(t, cmds, 100)
				for i := range 100 {
					command, ok := cmds[i].Command.Body.Payload.(AttackPlayerCommand)
					assert.True(t, ok)
					assert.Equal(t, i, command.Value)
				}
			},
		},
		{
			name: "get commands when multiple commands registered",
			setupFn: func(t *testing.T, cr *commandManager) {
				_, err := cr.register(AttackPlayerCommand{}.Name())
				require.NoError(t, err)

				_, err = cr.register(CreatePlayerCommand{}.Name())
				require.NoError(t, err)

				var commands1 [100]micro.Command
				for i := range 100 {
					commands1[i] = micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: AttackPlayerCommand{}.Name(), Payload: AttackPlayerCommand{Value: i}}}}
				}
				err = cr.receiveCommands(commands1[:])
				require.NoError(t, err)

				var commands2 [200]micro.Command
				for i := range 200 {
					commands2[i] = micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: CreatePlayerCommand{}.Name(), Payload: CreatePlayerCommand{Value: i}}}}
				}
				err = cr.receiveCommands(commands2[:])
				require.NoError(t, err)
			},
			queryName: CreatePlayerCommand{}.Name(),
			validateFn: func(t *testing.T, cmds []micro.Command) {
				assert.Len(t, cmds, 200)
				for i := range 200 {
					command, ok := cmds[i].Command.Body.Payload.(CreatePlayerCommand)
					assert.True(t, ok)
					assert.Equal(t, i, command.Value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cr := newCommandManager()
			tt.setupFn(t, &cr)

			cmds, err := cr.get(tt.queryName)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			tt.validateFn(t, cmds)
		})
	}
}

func TestCommandManager_Clear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setupFn func(*testing.T, *commandManager) []string
		testFn  func(*testing.T, *commandManager, []string)
	}{
		{
			name: "clears multiple command types with many commands",
			setupFn: func(t *testing.T, cr *commandManager) []string {
				_, err := cr.register(AttackPlayerCommand{}.Name())
				require.NoError(t, err)
				_, err = cr.register(CreatePlayerCommand{}.Name())
				require.NoError(t, err)

				var commands1 [50]micro.Command
				for i := range 50 {
					commands1[i] = micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: AttackPlayerCommand{}.Name(), Payload: AttackPlayerCommand{Value: i}}}}
				}
				var commands2 [100]micro.Command
				for i := range 100 {
					commands2[i] = micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: CreatePlayerCommand{}.Name(), Payload: CreatePlayerCommand{Value: i}}}}
				}

				// Add commands
				err = cr.receiveCommands(commands1[:])
				require.NoError(t, err)
				err = cr.receiveCommands(commands2[:])
				require.NoError(t, err)

				return []string{AttackPlayerCommand{}.Name(), CreatePlayerCommand{}.Name()}
			},
			testFn: func(t *testing.T, cr *commandManager, cmdNames []string) {
				for _, name := range cmdNames {
					cmds, err := cr.get(name)
					require.NoError(t, err)
					assert.Empty(t, cmds, "Commands for %s should be empty after clear", name)
				}
			},
		},
		{
			name: "can add commands after clearing",
			setupFn: func(t *testing.T, cr *commandManager) []string {
				_, err := cr.register(AttackPlayerCommand{}.Name())
				require.NoError(t, err)

				// Add multiple commands
				var commands [20]micro.Command
				for i := range 20 {
					commands[i] = micro.Command{Command: micro.CommandRaw{Body: micro.CommandBody{
						Name: AttackPlayerCommand{}.Name(), Payload: AttackPlayerCommand{Value: i}}}}
				}

				err = cr.receiveCommands(commands[:])
				require.NoError(t, err)

				return []string{AttackPlayerCommand{}.Name()}
			},
			testFn: func(t *testing.T, cr *commandManager, cmdNames []string) {
				cmds, err := cr.get(cmdNames[0])
				require.NoError(t, err)
				assert.Empty(t, cmds)

				err = cr.receiveCommands([]micro.Command{{Command: micro.CommandRaw{Body: micro.CommandBody{
					Name: AttackPlayerCommand{}.Name(), Payload: AttackPlayerCommand{}}}}})
				require.NoError(t, err)

				// Verify all new commands were added
				cmds, err = cr.get(cmdNames[0])
				require.NoError(t, err)
				assert.Len(t, cmds, 1)
			},
		},
		{
			name: "clear works on empty command lists",
			setupFn: func(t *testing.T, cr *commandManager) []string {
				_, err := cr.register(AttackPlayerCommand{}.Name())
				require.NoError(t, err)
				_, err = cr.register(CreatePlayerCommand{}.Name())
				require.NoError(t, err)
				return []string{AttackPlayerCommand{}.Name(), CreatePlayerCommand{}.Name()}
			},
			testFn: func(t *testing.T, cr *commandManager, cmdNames []string) {
				// Call clear on already empty commands
				cr.clear()

				// Verify still empty after clear
				for _, name := range cmdNames {
					cmds, err := cr.get(name)
					require.NoError(t, err)
					assert.Empty(t, cmds, "Commands for %s should remain empty after clear", name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cr := newCommandManager()
			cmdNames := tt.setupFn(t, &cr)

			cr.clear()

			tt.testFn(t, &cr, cmdNames)
		})
	}
}
