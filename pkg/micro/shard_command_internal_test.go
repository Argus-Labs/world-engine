package micro

// TODO: fix.

// import (
// 	"strings"
// 	"sync"
// 	"testing"
//
// 	"github.com/argus-labs/world-engine/pkg/cardinal/testutils"
// 	"github.com/argus-labs/world-engine/pkg/ecs"
// 	microtestutils "github.com/argus-labs/world-engine/pkg/micro/testutils"
// 	"github.com/argus-labs/world-engine/pkg/sign"
// 	"github.com/argus-labs/world-engine/pkg/telemetry"
// 	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/isc/v1"
// 	"github.com/goccy/go-json"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// 	"google.golang.org/protobuf/types/known/structpb"
// )
//
// func TestCommandManager_Has(t *testing.T) {
// 	t.Parallel()
//
// 	t.Run("returns true for registered command", func(t *testing.T) {
// 		t.Parallel()
// 		m := createTestCommandManager(t, ModeLeader)
// 		err := registerCommand[testutils.TestCommand](m)
// 		require.NoError(t, err)
// 		got := m.Has("test-command")
// 		assert.True(t, got, "Has should return true for registered command")
// 	})
//
// 	t.Run("returns false for unregistered command", func(t *testing.T) {
// 		t.Parallel()
// 		m := createTestCommandManager(t, ModeLeader)
// 		got := m.Has("test-command")
// 		assert.False(t, got, "Has should return false for unregistered command")
// 	})
//
// 	t.Run("returns true after multiple registrations", func(t *testing.T) {
// 		t.Parallel()
// 		m := createTestCommandManager(t, ModeLeader)
// 		err := registerCommand[testutils.TestCommand](m)
// 		require.NoError(t, err)
// 		err = registerCommand[testutils.TestCommand](m) // Second registration should be idempotent
// 		require.NoError(t, err, "Second registration should be idempotent")
// 		got := m.Has("test-command")
// 		assert.True(t, got, "Has should return true after registrations")
// 	})
// }
//
// func TestCommandManager_Enqueue(t *testing.T) {
// 	t.Parallel()
//
// 	t.Run("bulk enqueue same command", func(t *testing.T) {
// 		t.Parallel()
// 		m := createTestCommandManager(t, ModeLeader)
// 		err := registerCommand[testutils.TestCommand](m)
// 		require.NoError(t, err)
//
// 		// Enqueue 100 commands
// 		for i := range 100 {
// 			cmd := testutils.TestCommand{Value: i}
// 			signedCmd := createAndSignCommand(t, cmd)
// 			require.NoError(t, m.Enqueue(signedCmd))
// 		}
//
// 		// Verify all commands were enqueued
// 		tickData := m.GetTickData()
//
// 		require.Len(t, tickData.Commands, 100)
// 		for i, cmd := range tickData.Commands {
// 			testCmd, ok := cmd.Command.Body.Payload.(testutils.TestCommand)
// 			assert.True(t, ok)
// 			assert.Equal(t, i, testCmd.Value)
// 		}
// 	})
//
// 	t.Run("enqueue to unregistered command returns error", func(t *testing.T) {
// 		t.Parallel()
// 		m := createTestCommandManager(t, ModeLeader)
// 		cmd := testutils.TestCommand{Value: 42}
// 		signedCmd := createAndSignCommand(t, cmd)
//
// 		// Test should return error when trying to enqueue an unregistered command
// 		err := m.Enqueue(signedCmd)
// 		require.Error(t, err)
// 	})
//
// 	t.Run("concurrent multi-type commands", func(t *testing.T) {
// 		t.Parallel()
// 		m := createTestCommandManager(t, ModeLeader)
// 		err := registerCommand[testutils.TestCommand](m)
// 		require.NoError(t, err)
// 		err = registerCommand[testutils.AnotherTestCommand](m)
// 		require.NoError(t, err)
//
// 		const numGoroutines = 10
// 		const commandsPerGoroutine = 10
// 		var wg sync.WaitGroup
//
// 		// Launch multiple goroutines that enqueue different command types
// 		for i := range numGoroutines {
// 			wg.Add(1)
// 			go func(goroutineID int) {
// 				defer wg.Done()
// 				for j := range commandsPerGoroutine {
// 					// Alternate between command types
// 					if goroutineID%2 == 0 {
// 						cmd := testutils.TestCommand{Value: goroutineID*commandsPerGoroutine + j}
// 						signedCmd := createAndSignCommand(t, cmd)
// 						assert.NoError(t, m.Enqueue(signedCmd))
// 					} else {
// 						cmd := testutils.AnotherTestCommand{Value: goroutineID*commandsPerGoroutine + j}
// 						signedCmd := createAndSignCommand(t, cmd)
// 						assert.NoError(t, m.Enqueue(signedCmd))
// 					}
// 				}
// 			}(i)
// 		}
//
// 		wg.Wait()
//
// 		// Verify all commands were enqueued
// 		tickData := m.GetTickData()
//
// 		expectedTotal := numGoroutines * commandsPerGoroutine
// 		require.Len(t, tickData.Commands, expectedTotal)
//
// 		// Count commands of each type
// 		testCmdCount := 0
// 		anotherCmdCount := 0
// 		for _, cmd := range tickData.Commands {
// 			switch cmd.Command.Body.Payload.(type) {
// 			case testutils.TestCommand:
// 				testCmdCount++
// 			case testutils.AnotherTestCommand:
// 				anotherCmdCount++
// 			default:
// 				t.Errorf("unexpected command type: %T", cmd.Command.Body.Payload)
// 			}
// 		}
//
// 		// Verify we got the expected number of each command type
// 		// Half the goroutines use each type
// 		expectedPerType := (numGoroutines / 2) * commandsPerGoroutine
// 		assert.Equal(t, expectedPerType, testCmdCount)
// 		assert.Equal(t, expectedPerType, anotherCmdCount)
// 	})
// }
//
// func TestCommandManager_GetTickData(t *testing.T) {
// 	t.Parallel()
//
// 	tests := []struct {
// 		name   string
// 		setup  func(*commandManager)
// 		verify func(*testing.T, TickData)
// 	}{
// 		{
// 			name: "empty manager returns empty slice",
// 			verify: func(t *testing.T, tickData TickData) {
// 				assert.Empty(t, tickData.Commands)
// 				assert.NotNil(t, tickData.Commands)
// 			},
// 		},
// 		{
// 			name: "get commands from single channel",
// 			setup: func(m *commandManager) {
// 				err := registerCommand[testutils.TestCommand](m)
// 				require.NoError(t, err)
// 				for i := range 5 {
// 					cmd := testutils.TestCommand{Value: i}
// 					signedCmd := createAndSignCommand(t, cmd)
// 					require.NoError(t, m.Enqueue(signedCmd))
// 				}
// 			},
// 			verify: func(t *testing.T, tickData TickData) {
// 				require.Len(t, tickData.Commands, 5)
// 				for i, cmd := range tickData.Commands {
// 					testCmd, ok := cmd.Command.Body.Payload.(testutils.TestCommand)
// 					assert.True(t, ok)
// 					assert.Equal(t, i, testCmd.Value)
// 				}
// 			},
// 		},
// 		{
// 			name: "get commands from multiple channels",
// 			setup: func(m *commandManager) {
// 				err := registerCommand[testutils.TestCommand](m)
// 				require.NoError(t, err)
// 				err = registerCommand[testutils.AnotherTestCommand](m)
// 				require.NoError(t, err)
// 				for i := range 3 {
// 					cmd1 := testutils.TestCommand{Value: i}
// 					cmd2 := testutils.AnotherTestCommand{Value: i + 100}
// 					signedCmd1 := createAndSignCommand(t, cmd1)
// 					signedCmd2 := createAndSignCommand(t, cmd2)
// 					require.NoError(t, m.Enqueue(signedCmd1))
// 					require.NoError(t, m.Enqueue(signedCmd2))
// 				}
// 			},
// 			verify: func(t *testing.T, tickData TickData) {
// 				require.Len(t, tickData.Commands, 6)
// 				testCmdCount := 0
// 				anotherCmdCount := 0
// 				for _, cmd := range tickData.Commands {
// 					switch payload := cmd.Command.Body.Payload.(type) {
// 					case testutils.TestCommand:
// 						assert.Equal(t, testCmdCount, payload.Value)
// 						testCmdCount++
// 					case testutils.AnotherTestCommand:
// 						assert.Equal(t, anotherCmdCount+100, payload.Value)
// 						anotherCmdCount++
// 					default:
// 						t.Errorf("unexpected command type: %T", payload)
// 					}
// 				}
// 				assert.Equal(t, 3, testCmdCount)
// 				assert.Equal(t, 3, anotherCmdCount)
// 			},
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			t.Parallel()
// 			m := createTestCommandManager(t, ModeLeader)
// 			if tt.setup != nil {
// 				tt.setup(m)
// 			}
//
// 			tickData := m.GetTickData()
// 			tt.verify(t, tickData)
// 		})
// 	}
// }
//
// func TestRegisterCommand_ModeBasedBehavior(t *testing.T) {
// 	t.Parallel()
//
// 	t.Run("follower mode does not create endpoints", func(t *testing.T) {
// 		t.Parallel()
// 		m := createTestCommandManager(t, ModeFollower)
//
// 		// Register command in follower mode
// 		err := registerCommand[testutils.TestCommand](m)
// 		require.NoError(t, err)
//
// 		// Command should be registered in the channel map
// 		assert.True(t, m.Has("test-command"), "Command should be registered in follower mode")
//
// 		// But no endpoint should be created in the service
// 		endpointName := "command.test-command"
// 		_, exists := m.endpoints[endpointName]
// 		assert.False(t, exists, "No endpoint should be created in follower mode")
// 	})
//
// 	t.Run("leader mode creates endpoints", func(t *testing.T) {
// 		t.Parallel()
// 		m := createTestCommandManager(t, ModeLeader)
//
// 		// Register command in leader mode
// 		err := registerCommand[testutils.TestCommand](m)
// 		require.NoError(t, err)
//
// 		// Command should be registered in the channel map
// 		assert.True(t, m.Has("test-command"), "Command should be registered in leader mode")
//
// 		// Endpoint should be created in the service
// 		endpointName := "command.test-command"
// 		_, exists := m.endpoints[endpointName]
// 		assert.True(t, exists, "Endpoint should be created in leader mode")
// 	})
// }
//
// func createTestCommandManager(t *testing.T, mode ShardMode) *commandManager {
// 	t.Helper()
//
// 	// Create a new NATS server for each test
// 	nats := microtestutils.NewNATS(t)
//
// 	// Create a test client with NATS URL pointing to our test server
// 	client, err := NewTestClient(nats.Server.ClientURL())
// 	require.NoError(t, err)
//
// 	// Create command manager with telemetry
// 	tel, err := telemetry.New(telemetry.Options{ServiceName: "test-command-manager"})
// 	require.NoError(t, err)
//
// 	opts := ShardOptions{
// 		Client:         client,
// 		Address:        GetAddress(RealmInternal, "test-org", "test-proj", "test-id"),
// 		Mode:           mode,
// 		EpochFrequency: 10,
// 		TickRate:       1,
// 		Telemetry:      &tel,
// 	}
//
// 	testShard, err := NewShard(testShardEngine{}, opts)
// 	require.NoError(t, err)
//
// 	m, err := newCommandManager(testShard, opts)
// 	require.NoError(t, err)
//
// 	return &m
// }
//
// func createAndSignCommand(t *testing.T, cmd ecs.Command) *iscv1.Command {
// 	t.Helper()
//
// 	signer, err := sign.NewSigner(strings.Repeat("00", 32), 42)
// 	if err != nil {
// 		// Make it safe for use inside another goroutine.
// 		t.Errorf("failed to create signer: %v", err)
// 		return nil
// 	}
//
// 	// Marshal command to protobuf struct directly to avoid import cycle
// 	cmdBytes, err := json.Marshal(cmd)
// 	if err != nil {
// 		t.Errorf("failed to marshal test command to JSON: %v", err)
// 		return nil
// 	}
//
// 	var cmdMap map[string]any
// 	if err := json.Unmarshal(cmdBytes, &cmdMap); err != nil {
// 		t.Errorf("failed to unmarshal command to map: %v", err)
// 		return nil
// 	}
//
// 	pbStruct, err := structpb.NewStruct(cmdMap)
// 	if err != nil {
// 		t.Errorf("failed to create protobuf struct: %v", err)
// 		return nil
// 	}
//
// 	pbCommand := &iscv1.CommandBody{
// 		Name:    cmd.Name(),
// 		Payload: pbStruct,
// 	}
//
// 	signedCommand, err := signer.SignCommand(pbCommand, iscv1.AuthInfo_MODE_PERSONA)
// 	if err != nil {
// 		t.Errorf("failed to create signed command: %v", err)
// 		return nil
// 	}
//
// 	return signedCommand
// }
//
// type testShardEngine struct{}
//
// var _ ShardEngine = testShardEngine{}
//
// func (testShardEngine) Init() error              { return nil }
// func (testShardEngine) StateHash() []byte        { return nil }
// func (testShardEngine) Tick(tick Tick) error     { return nil }
// func (testShardEngine) Replay(replay Tick) error { return nil }
