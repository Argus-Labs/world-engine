package micro

// import (
// 	"testing"
//
// 	"github.com/argus-labs/world-engine/pkg/cardinal/protoutil"
// 	"github.com/argus-labs/world-engine/pkg/cardinal/testutils"
// 	microtestutils "github.com/argus-labs/world-engine/pkg/micro/testutils"
// 	"github.com/argus-labs/world-engine/pkg/sign"
// 	"github.com/argus-labs/world-engine/pkg/telemetry"
// 	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/isc/v1"
// 	microv1 "github.com/argus-labs/world-engine/proto/gen/go/micro/v1"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// )
//
// func TestCommandHandler(t *testing.T) {
// 	t.Parallel()
//
// 	tests := []struct {
// 		name             string
// 		setupManager     func(t *testing.T) *commandManager
// 		createCommand    func(t *testing.T) *iscv1.Command
// 		wantErr          bool
// 		errMsg           string
// 		verifyEnqueued   bool
// 		expectedCommands int
// 	}{
// 		{
// 			name: "successful command enqueue with valid payload",
// 			setupManager: func(t *testing.T) *commandManager {
// 				m := createTestCommandManagerWithShutdown(t, false)
// 				err := registerCommand[testutils.TestCommand](m)
// 				require.NoError(t, err)
// 				return m
// 			},
// 			createCommand:    createValidCommand,
// 			wantErr:          false,
// 			verifyEnqueued:   true,
// 			expectedCommands: 1,
// 		},
// 		{
// 			name: "unregistered command returns error",
// 			setupManager: func(t *testing.T) *commandManager {
// 				return createTestCommandManagerWithShutdown(t, false) // Don't register command
// 			},
// 			createCommand:    createValidCommand,
// 			wantErr:          true,
// 			errMsg:           "unregistered command",
// 			verifyEnqueued:   false,
// 			expectedCommands: 0,
// 		},
// 		{
// 			name: "command unmarshaling failure",
// 			setupManager: func(t *testing.T) *commandManager {
// 				m := createTestCommandManagerWithShutdown(t, false)
// 				err := registerCommand[testutils.TestCommand](m)
// 				require.NoError(t, err)
// 				return m
// 			},
// 			createCommand: func(t *testing.T) *iscv1.Command {
// 				return &iscv1.Command{
// 					CommandBytes: []byte("invalid-proto-bytes"),
// 					Signature:    make([]byte, 64),
// 					AuthInfo: &iscv1.AuthInfo{
// 						SignerAddress: make([]byte, 32),
// 					},
// 				}
// 			},
// 			wantErr:          true,
// 			errMsg:           "failed to unmarshal command bytes",
// 			verifyEnqueued:   false,
// 			expectedCommands: 0,
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			t.Parallel()
//
// 			manager := tt.setupManager(t)
// 			cmd := tt.createCommand(t)
//
// 			// Test direct enqueue (this is what the handler calls)
// 			err := manager.Enqueue(cmd)
//
// 			if tt.wantErr {
// 				require.Error(t, err)
// 				assert.Contains(t, err.Error(), tt.errMsg)
// 			} else {
// 				require.NoError(t, err)
// 			}
//
// 			// Verify enqueue state
// 			tickData := manager.GetTickData()
// 			assert.Len(t, tickData.Commands, tt.expectedCommands)
//
// 			if tt.verifyEnqueued && tt.expectedCommands > 0 {
// 				// Verify the command type and content
// 				command, ok := tickData.Commands[0].Command.Body.Payload.(testutils.TestCommand)
// 				assert.True(t, ok)
// 				assert.Equal(t, 42, command.Value)
// 			}
// 		})
// 	}
// }
//
// // createTestCommandManagerWithShutdown creates a test command manager with configurable shutdown state.
// func createTestCommandManagerWithShutdown(
// 	t *testing.T,
// 	closed bool, //nolint:unparam // tests that use this aren't impelemented yet
// ) *commandManager {
// 	t.Helper()
//
// 	// Create a test NATS server and client
// 	natsServer := microtestutils.NewNATS(t)
//
// 	// Create a test client with NATS URL pointing to our test server
// 	client, err := NewClient(WithNATSConfig(NATSConfig{
// 		URL: natsServer.Server.ClientURL(),
// 	}))
// 	require.NoError(t, err)
//
// 	// Create a test service address
// 	address := &ServiceAddress{
// 		Realm:        microv1.ServiceAddress_REALM_INTERNAL,
// 		Organization: "test-org",
// 		Project:      "test-project",
// 		ServiceId:    "test-service",
// 	}
//
// 	// Create command manager with telemetry
// 	tel, err := telemetry.New(telemetry.Options{ServiceName: "test-command-manager"})
// 	require.NoError(t, err)
// 	m, err := newCommandManager(ShardOptions{
// 		Client:    client,
// 		Address:   address,
// 		Mode:      ModeFollower,
// 		Telemetry: &tel,
// 	})
// 	require.NoError(t, err)
//
// 	return &m
// }
//
// // createValidCommand helper creates a valid signed command for testing.
// func createValidCommand(t *testing.T) *iscv1.Command {
// 	cmd := testutils.TestCommand{Value: 42}
//
// 	// Use protoutil to marshal command
// 	pbCommand, err := protoutil.MarshalCommand(cmd)
// 	require.NoError(t, err)
//
// 	// Create a test signer for generating valid signatures
// 	signer, err := sign.NewSigner("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", 0)
// 	require.NoError(t, err)
//
// 	// Add required fields for the command body
// 	pbCommand.Persona = &iscv1.Persona{Id: "test-persona"}
// 	pbCommand.Address = &microv1.ServiceAddress{
// 		Realm:        microv1.ServiceAddress_REALM_WORLD,
// 		Organization: "test-org",
// 		Project:      "test-project",
// 		ServiceId:    "test-service",
// 	}
//
// 	// Use actual signing functionality to create properly signed command
// 	signedCmd, err := signer.SignCommand(pbCommand, iscv1.AuthInfo_MODE_PERSONA)
// 	require.NoError(t, err)
//
// 	return signedCmd
// }
//
// func TestCommandRegistration(t *testing.T) {
// 	t.Parallel()
//
// 	t.Run("command is registered correctly", func(t *testing.T) {
// 		t.Parallel()
// 		manager := createTestCommandManagerWithShutdown(t, false)
//
// 		// Register a command
// 		err := registerCommand[testutils.TestCommand](manager)
// 		require.NoError(t, err)
//
// 		// Verify the command is registered
// 		assert.True(t, manager.Has("test-command"))
// 	})
//
// 	t.Run("follower mode registers command channel", func(t *testing.T) {
// 		t.Parallel()
// 		manager := createTestCommandManagerWithShutdown(t, false)
//
// 		// Register a command in follower mode
// 		err := registerCommand[testutils.TestCommand](manager)
// 		require.NoError(t, err)
//
// 		// Verify the command channel is registered
// 		assert.True(t, manager.Has("test-command"))
// 	})
//
// 	t.Run("double registration fails for leader mode", func(t *testing.T) {
// 		t.Parallel()
// 		manager := createTestCommandManagerWithShutdown(t, false)
//
// 		// First registration should succeed
// 		err := registerCommand[testutils.TestCommand](manager)
// 		require.NoError(t, err)
//
// 		// Second registration should fail due to endpoint conflict
// 		err = registerCommand[testutils.TestCommand](manager)
// 		require.Error(t, err)
// 		assert.Contains(t, err.Error(), "endpoint already exists")
// 	})
//
// 	t.Run("double registration succeeds for follower mode", func(t *testing.T) {
// 		t.Parallel()
// 		manager := createTestCommandManagerWithShutdown(t, false)
//
// 		// First registration should succeed
// 		err := registerCommand[testutils.TestCommand](manager)
// 		require.NoError(t, err)
//
// 		// Second registration should also succeed (no endpoints registered)
// 		err = registerCommand[testutils.TestCommand](manager)
// 		require.NoError(t, err)
// 	})
// }
//
// // TestCommandVerification tests the command verification functionality.
// // This is currently a stub test since verification is commented out in the handler.
// func TestCommandVerification(t *testing.T) {
// 	t.Parallel()
//
// 	t.Run("command verification stub", func(t *testing.T) {
// 		t.Parallel()
//
// 		// TODO: This test is a stub for command verification functionality
// 		// that is currently commented out in registerCommand (lines 112-115).
// 		// When verification is re-enabled, this test should be expanded to cover:
// 		//
// 		// 1. Valid signature verification passes
// 		// 2. Invalid signature verification fails
// 		// 3. Missing signature verification fails
// 		// 4. Expired signature verification fails
// 		// 5. Signature from unregistered persona fails
// 		//
// 		// The verification logic should call something like:
// 		// if err := s.personas.VerifySignedCommand(command, true); err != nil {
// 		//     return handleSpanError(span, req, eris.Wrap(err, "failed to verify signed command"))
// 		// }
//
// 		manager := createTestCommandManagerWithShutdown(t, false)
// 		err := registerCommand[testutils.TestCommand](manager)
// 		require.NoError(t, err)
//
// 		// Create a valid command
// 		cmd := createValidCommand(t)
//
// 		// For now, just test that enqueue works without verification
// 		err = manager.Enqueue(cmd)
// 		require.NoError(t, err, "Command should enqueue successfully without verification")
//
// 		// Verify command was enqueued
// 		tickData := manager.GetTickData()
// 		assert.Len(t, tickData.Commands, 1)
//
// 		// When verification is re-enabled, add tests like:
// 		// t.Run("invalid signature fails verification", func(t *testing.T) { ... })
// 		// t.Run("expired signature fails verification", func(t *testing.T) { ... })
// 		// etc.
// 	})
//
// 	t.Run("verification placeholder - invalid signature", func(t *testing.T) {
// 		t.Parallel()
//
// 		// TODO: Implement when verification is re-enabled
// 		// This should test that commands with invalid signatures are rejected
// 		t.Skip("Command verification is currently disabled - implement when re-enabled")
// 	})
//
// 	t.Run("verification placeholder - expired signature", func(t *testing.T) {
// 		t.Parallel()
//
// 		// TODO: Implement when verification is re-enabled
// 		// This should test that commands with expired signatures are rejected
// 		t.Skip("Command verification is currently disabled - implement when re-enabled")
// 	})
//
// 	t.Run("verification placeholder - unregistered persona", func(t *testing.T) {
// 		t.Parallel()
//
// 		// TODO: Implement when verification is re-enabled
// 		// This should test that commands from unregistered personas are rejected
// 		t.Skip("Command verification is currently disabled - implement when re-enabled")
// 	})
// }
