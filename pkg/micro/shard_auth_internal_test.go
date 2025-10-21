package micro

// TODO: fix.

// import (
// 	"crypto/ed25519"
// 	"encoding/hex"
// 	"strings"
// 	"testing"
// 	"time"
//
// 	microtestutils "github.com/argus-labs/world-engine/pkg/micro/testutils"
// 	"github.com/argus-labs/world-engine/pkg/sign"
// 	"github.com/argus-labs/world-engine/pkg/telemetry"
// 	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/isc/v1"
// 	microv1 "github.com/argus-labs/world-engine/proto/gen/go/micro/v1"
// 	registryv1 "github.com/argus-labs/world-engine/proto/gen/go/registry/v1"
// 	"github.com/cosmos/iavl"
// 	"github.com/cosmos/iavl/db"
// 	ics23 "github.com/cosmos/ics23/go"
//
// 	"github.com/nats-io/nats.go"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
//
// 	"google.golang.org/genproto/googleapis/rpc/status"
// 	"google.golang.org/protobuf/proto"
// 	"google.golang.org/protobuf/types/known/anypb"
// 	"google.golang.org/protobuf/types/known/structpb"
// 	"google.golang.org/protobuf/types/known/timestamppb"
// )
//
// // TestCommandVerifier_VerifyCommand tests the initial validation steps before authentication modes.
// // This includes command unmarshalling, address validation, and TTL validation.
// func TestCommandVerifier_VerifyCommand(t *testing.T) {
// 	t.Parallel()
//
// 	tests := []struct {
// 		name          string
// 		mode          ShardMode
// 		commandFunc   func(t *testing.T, address *ServiceAddress) *iscv1.Command
// 		expectError   bool
// 		errorContains string
// 	}{
// 		// Step 1: Command bytes unmarshalling
// 		{
// 			name: "Should fail when command bytes cannot be unmarshalled",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				return &iscv1.Command{
// 					CommandBytes: []byte("invalid protobuf data"),
// 					AuthInfo: &iscv1.AuthInfo{
// 						Mode: iscv1.AuthInfo_MODE_DIRECT,
// 					},
// 					Signature: []byte("signature"),
// 				}
// 			},
// 			expectError:   true,
// 			errorContains: "failed to unmarshal command bytes",
// 		},
// 		{
// 			name: "Should fail when raw command fails proto validation",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				// Create an invalid command raw (missing required fields)
// 				commandRaw := &iscv1.CommandRaw{
// 					// Missing timestamp - this should fail validation
// 					Body: &iscv1.CommandBody{
// 						Name:    "test-command",
// 						Address: address,
// 					},
// 				}
//
// 				commandBytes, err := proto.Marshal(commandRaw)
// 				require.NoError(t, err)
//
// 				return &iscv1.Command{
// 					CommandBytes: commandBytes,
// 					AuthInfo: &iscv1.AuthInfo{
// 						Mode: iscv1.AuthInfo_MODE_DIRECT,
// 					},
// 					Signature: []byte("signature"),
// 				}
// 			},
// 			expectError:   true,
// 			errorContains: "failed to validate raw command",
// 		},
//
// 		// Step 2: Address matching
// 		{
// 			name: "Should fail when command address doesn't match shard address",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				wrongAddress := GetAddress(RealmInternal, "wrong-org", "wrong-project", "wrong-shard")
// 				return createValidCommand(t, wrongAddress, iscv1.AuthInfo_MODE_DIRECT, time.Now())
// 			},
// 			expectError:   true,
// 			errorContains: "command address doesn't match shard address",
// 		},
//
// 		// Step 3: TTL validation (leader mode only)
// 		{
// 			name: "Should fail when command has expired in leader mode",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				expiredTime := time.Now().Add(-maxCommandTTL - time.Minute)
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_DIRECT, expiredTime)
// 			},
// 			expectError:   true,
// 			errorContains: "command has expired",
// 		},
// 		{
// 			name: "Should fail when command timestamp is too far in the future in leader mode",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				futureTime := time.Now().Add(clockDriftTolerance + time.Minute)
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_DIRECT, futureTime)
// 			},
// 			expectError:   true,
// 			errorContains: "command timestamp is more than",
// 		},
// 		{
// 			name: "Should fail when replay attack is detected in leader mode",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_DIRECT, time.Now())
// 			},
// 			expectError:   true,
// 			errorContains: "replay attack detected",
// 		},
// 		{
// 			name: "Should skip TTL validation in follower mode",
// 			mode: ModeFollower,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				expiredTime := time.Now().Add(-maxCommandTTL - time.Minute)
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_DIRECT, expiredTime)
// 			},
// 			expectError: false,
// 		},
// 		// Step 4: Unspecified auth mode
// 		{
// 			name: "Should fail when auth mode is unspecified",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_UNSPECIFIED, time.Now())
// 			},
// 			expectError:   true,
// 			errorContains: "unspecified command auth mode",
// 		},
// 	}
//
// 	natsServer := microtestutils.NewNATS(t)
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			t.Parallel()
//
// 			address := GetAddress(RealmInternal, "test-org", "test-project", "test-shard")
// 			verifier := createTestCommandVerifier(t, address, tt.mode, natsServer)
// 			command := tt.commandFunc(t, address)
//
// 			// Cleanup shard resources
// 			t.Cleanup(func() {
// 				verifier.client.Close()
// 			})
//
// 			// Special handling for replay attack test
// 			if strings.Contains(tt.name, "replay attack") {
// 				expirySeconds := int((maxCommandTTL + cacheRetentionExtra).Seconds())
// 				err := verifier.cache.Set(command.GetSignature(), []byte{}, expirySeconds)
// 				require.NoError(t, err)
// 			}
//
// 			err := verifier.VerifyCommand(command)
//
// 			if tt.expectError {
// 				require.Error(t, err)
// 				if tt.errorContains != "" {
// 					assert.Contains(t, err.Error(), tt.errorContains)
// 				}
// 			} else {
// 				require.NoError(t, err)
// 			}
// 		})
// 	}
// }
//
// func TestCommandVerifier_VerifyCommand_DirectMode(t *testing.T) {
// 	t.Parallel()
//
// 	tests := []struct {
// 		name          string
// 		mode          ShardMode
// 		commandFunc   func(t *testing.T, address *ServiceAddress) *iscv1.Command
// 		expectError   bool
// 		errorContains string
// 	}{
// 		{
// 			name: "Should fail direct mode authentication with invalid signature",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				// Create a command with an invalid signature
// 				command := createValidCommand(t, address, iscv1.AuthInfo_MODE_DIRECT, time.Now())
// 				command.Signature = []byte("invalid-signature-bytes")
// 				return command
// 			},
// 			expectError:   true,
// 			errorContains: "invalid signature",
// 		},
// 		{
// 			name: "Should succeed direct mode authentication with valid signature in leader mode",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				// Create a properly signed command for direct mode
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_DIRECT, time.Now())
// 			},
// 			expectError: false,
// 		},
// 		{
// 			name: "Should succeed direct mode authentication with valid signature in follower mode",
// 			mode: ModeFollower,
// 			commandFunc: func(t *testing.T, address *ServiceAddress) *iscv1.Command {
// 				// Create a properly signed command for direct mode
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_DIRECT, time.Now())
// 			},
// 			expectError: false,
// 		},
// 	}
//
// 	natsServer := microtestutils.NewNATS(t)
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			t.Parallel()
//
// 			address := GetAddress(RealmInternal, "test-org", "test-project", "test-shard")
// 			verifier := createTestCommandVerifier(t, address, tt.mode, natsServer)
// 			command := tt.commandFunc(t, address)
//
// 			// Cleanup shard resources
// 			t.Cleanup(func() {
// 				verifier.client.Close()
// 			})
//
// 			err := verifier.VerifyCommand(command)
//
// 			if tt.expectError {
// 				require.Error(t, err)
// 				if tt.errorContains != "" {
// 					assert.Contains(t, err.Error(), tt.errorContains)
// 				}
// 			} else {
// 				require.NoError(t, err)
// 			}
// 		})
// 	}
// }
//
// const testPersonaID = "test-persona"
// const testSignerAddress = "0000000000000000000000000000000000000000000000000000000000000000"
//
// func TestCommandVerifier_VerifyCommand_PersonaMode(t *testing.T) {
// 	// t.Parallel()
//
// 	tests := []struct {
// 		name          string
// 		mode          ShardMode
// 		setupFunc     func(t *testing.T, tree *iavl.MutableTree, verifier *commandVerifier)
// 		commandFunc   func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command
// 		replyFunc     func(*testing.T, *registryv1.QueryPersonaRequest, *iavl.MutableTree) *registryv1.QueryPersonaResponse
// 		expectError   bool
// 		errorContains string
// 	}{
// 		{
// 			name: "Should fail persona mode authentication with invalid signature",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				// Create a valid command then corrupt the signature
// 				command := createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 				command.Signature = []byte("invalid-signature-bytes")
// 				return command
// 			},
// 			replyFunc:     registryShouldNotBeCalledReplyFunc,
// 			expectError:   true,
// 			errorContains: "invalid signature",
// 		},
// 		{
// 			name: "Should succeed persona mode authentication using cached persona",
// 			mode: ModeLeader,
// 			setupFunc: func(t *testing.T, tree *iavl.MutableTree, verifier *commandVerifier) {
// 				signer, err := sign.NewSigner(testSignerAddress, 42)
// 				require.NoError(t, err)
//
// 				hash, version, proof := addPersonaToTree(t, tree)
// 				verifier.personas[testPersonaID] = personaWithMerkleProof{
// 					ID:      testPersonaID,
// 					Signers: [][]byte{signer.GetSignerAddress()},
// 					TTL:     time.Now().Add(time.Hour),
// 					Version: version,
// 					Proof:   proof,
// 					Root:    hash,
// 				}
// 			},
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 			},
// 			replyFunc:   registryShouldNotBeCalledReplyFunc,
// 			expectError: false,
// 		},
// 		{
// 			name: "Should fetch persona from registry when cache is expired",
// 			mode: ModeLeader,
// 			setupFunc: func(t *testing.T, tree *iavl.MutableTree, verifier *commandVerifier) {
// 				signer, err := sign.NewSigner(testSignerAddress, 42)
// 				require.NoError(t, err)
//
// 				// Pre-populate cache with expired persona (so it will fetch from registry)
// 				hash, version, _ := addPersonaToTree(t, tree)
// 				verifier.personas[testPersonaID] = personaWithMerkleProof{
// 					ID:      testPersonaID,
// 					Signers: [][]byte{signer.GetSignerAddress()},
// 					TTL:     time.Now().Add(-time.Hour), // Expired
// 					Version: version,
// 					Proof:   nil,
// 					Root:    hash,
// 				}
// 			},
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 			},
// 			replyFunc: func(
// 				t *testing.T,
// 				req *registryv1.QueryPersonaRequest,
// 				tree *iavl.MutableTree,
// 			) *registryv1.QueryPersonaResponse {
// 				return buildPersonaResponse(t, tree)
// 			},
// 			expectError: false,
// 		},
// 		{
// 			name: "Should fetch persona from registry when not in cache",
// 			mode: ModeLeader,
// 			setupFunc: func(t *testing.T, tree *iavl.MutableTree, verifier *commandVerifier) {
// 				addPersonaToTree(t, tree)
// 			},
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 			},
// 			replyFunc: func(
// 				t *testing.T,
// 				req *registryv1.QueryPersonaRequest,
// 				tree *iavl.MutableTree,
// 			) *registryv1.QueryPersonaResponse {
// 				return buildPersonaResponse(t, tree)
// 			},
// 			expectError: false,
// 		},
// 		{
// 			name: "Should fail when registry response cannot be unmarshalled",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 			},
// 			replyFunc: func(
// 				t *testing.T,
// 				req *registryv1.QueryPersonaRequest,
// 				tree *iavl.MutableTree,
// 			) *registryv1.QueryPersonaResponse {
// 				// Return invalid protobuf data that can't be unmarshalled by sending corrupted response
// 				return nil // This will cause unmarshalling to fail
// 			},
// 			expectError:   true,
// 			errorContains: "validation error",
// 		},
// 		{
// 			name: "Should fail when registry response fails validation",
// 			mode: ModeLeader,
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 			},
// 			replyFunc: func(
// 				t *testing.T,
// 				req *registryv1.QueryPersonaRequest,
// 				tree *iavl.MutableTree,
// 			) *registryv1.QueryPersonaResponse {
// 				// Return response with fields that will fail protobuf validation rules
// 				return &registryv1.QueryPersonaResponse{
// 					Details:   nil, // Missing required Details field
// 					ExpiresAt: nil, // Missing required ExpiresAt field
// 				}
// 			},
// 			expectError:   true,
// 			errorContains: "validation error",
// 		},
// 		{
// 			name: "Should fail when merkle proof verification fails",
// 			mode: ModeLeader,
// 			setupFunc: func(t *testing.T, tree *iavl.MutableTree, verifier *commandVerifier) {
// 				addPersonaToTree(t, tree)
// 			},
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 			},
// 			replyFunc: func(
// 				t *testing.T,
// 				req *registryv1.QueryPersonaRequest,
// 				tree *iavl.MutableTree,
// 			) *registryv1.QueryPersonaResponse {
// 				response := buildPersonaResponse(t, tree)
// 				response.Root[0] ^= 1
// 				return response
// 			},
// 			expectError:   true,
// 			errorContains: "invalid merkle proof",
// 		},
// 		{
// 			name: "Should fail when signer is not authorized for persona",
// 			mode: ModeLeader,
// 			setupFunc: func(t *testing.T, tree *iavl.MutableTree, verifier *commandVerifier) {
// 				// Setup persona with unauthorized signer (not the test signer)
// 				unauthorizedSigner := make([]byte, 32)
// 				for i := range unauthorizedSigner {
// 					unauthorizedSigner[i] = 0xFF // Different from test signer
// 				}
//
// 				details := PersonaDetails{Signers: [][]byte{unauthorizedSigner}}
// 				detailsBytes, err := details.Marshal()
// 				require.NoError(t, err)
//
// 				_, err = tree.Set([]byte(testPersonaID), detailsBytes)
// 				require.NoError(t, err)
//
// 				_, _, err = tree.SaveVersion()
// 				require.NoError(t, err)
// 			},
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 			},
// 			replyFunc: func(
// 				t *testing.T,
// 				req *registryv1.QueryPersonaRequest,
// 				tree *iavl.MutableTree,
// 			) *registryv1.QueryPersonaResponse {
// 				// buildPersonaResponse will read the unauthorized signer from tree and create valid proof
// 				return buildPersonaResponse(t, tree)
// 			},
// 			expectError:   true,
// 			errorContains: "is not an authorized signer for persona",
// 		},
// 		{
// 			name: "Should succeed persona mode authentication in leader mode",
// 			mode: ModeLeader,
// 			setupFunc: func(t *testing.T, tree *iavl.MutableTree, verifier *commandVerifier) {
// 				addPersonaToTree(t, tree)
// 			},
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 			},
// 			replyFunc: func(
// 				t *testing.T,
// 				req *registryv1.QueryPersonaRequest,
// 				tree *iavl.MutableTree,
// 			) *registryv1.QueryPersonaResponse {
// 				return buildPersonaResponse(t, tree)
// 			},
// 			expectError: false,
// 		},
// 		{
// 			name: "Should succeed persona mode authentication in follower mode",
// 			mode: ModeFollower,
// 			setupFunc: func(t *testing.T, tree *iavl.MutableTree, verifier *commandVerifier) {
// 				addPersonaToTree(t, tree)
// 			},
// 			commandFunc: func(t *testing.T, address *ServiceAddress, verifier *commandVerifier) *iscv1.Command {
// 				return createValidCommand(t, address, iscv1.AuthInfo_MODE_PERSONA, time.Now())
// 			},
// 			replyFunc: func(
// 				t *testing.T,
// 				req *registryv1.QueryPersonaRequest,
// 				tree *iavl.MutableTree,
// 			) *registryv1.QueryPersonaResponse {
// 				return buildPersonaResponse(t, tree)
// 			},
// 			expectError: false,
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// t.Parallel()
//
// 			// Step 1: Initialize registry tree and verifier
// 			tree := iavl.NewMutableTree(db.NewMemDB(), 128, false, iavl.NewNopLogger())
// 			address := GetAddress(RealmInternal, "test-org", "test-project", "test-shard")
// 			verifier := createTestCommandVerifier(t, address, tt.mode, nil)
//
// 			// Step 2: Setup registry state
// 			if tt.setupFunc != nil {
// 				tt.setupFunc(t, tree, verifier)
// 			}
//
// 			// Step 3: Setup command
// 			command := tt.commandFunc(t, address, verifier)
//
// 			// Step 4: Setup NATS handler for registry queries
// 			registryAddress := GetAddress(RealmInternal, "argus", "platform", "registry")
// 			subject := Endpoint(registryAddress, "query.persona")
//
// 			_, err := verifier.client.Subscribe(subject, func(msg *nats.Msg) {
// 				// Unmarshal the incoming request
// 				var request microv1.Request
// 				err := proto.Unmarshal(msg.Data, &request)
// 				require.NoError(t, err)
//
// 				var queryRequest registryv1.QueryPersonaRequest
// 				err = request.GetPayload().UnmarshalTo(&queryRequest)
// 				require.NoError(t, err)
//
// 				// Get the response from the test's replyFunc
// 				if tt.replyFunc == nil {
// 					// Skip for unimplemented test cases
// 					return
// 				}
// 				queryResponse := tt.replyFunc(t, &queryRequest, tree)
//
// 				// Marshal the response
// 				payload, err := anypb.New(queryResponse)
// 				require.NoError(t, err)
//
// 				response := &microv1.Response{
// 					Status:  &status.Status{Code: 0},
// 					Payload: payload,
// 				}
//
// 				responseBytes, err := proto.Marshal(response)
// 				require.NoError(t, err)
//
// 				err = msg.Respond(responseBytes)
// 				require.NoError(t, err)
// 			})
// 			require.NoError(t, err)
//
// 			// Step 5: Execute the test
// 			err = verifier.VerifyCommand(command)
//
// 			// Step 6: Check if the test statement is correct
// 			if tt.expectError {
// 				require.Error(t, err)
// 				if tt.errorContains != "" {
// 					assert.Contains(t, err.Error(), tt.errorContains)
// 				}
// 			} else {
// 				require.NoError(t, err)
// 			}
// 		})
// 	}
// }
//
// // -------------------------------------------------------------------------------------------------
// // Helper functions
// // -------------------------------------------------------------------------------------------------
//
// func createTestCommandVerifier(
// 	t *testing.T, address *ServiceAddress, mode ShardMode, natsServer *microtestutils.NATS,
// ) *commandVerifier {
// 	t.Helper()
//
// 	if natsServer == nil {
// 		natsServer = microtestutils.NewNATS(t)
// 	}
// 	client, err := NewTestClient(natsServer.Server.ClientURL())
// 	require.NoError(t, err)
//
// 	tel, err := telemetry.New(telemetry.Options{ServiceName: "test-verifier"})
// 	require.NoError(t, err)
//
// 	opts := ShardOptions{
// 		Client:         client,
// 		Address:        address,
// 		Mode:           mode,
// 		EpochFrequency: 10,
// 		TickRate:       1,
// 		Telemetry:      &tel,
// 	}
//
// 	shard, err := NewShard(testShardEngine{}, opts)
// 	require.NoError(t, err)
//
// 	verifier := newCommandVerifer(shard, 128, address, client)
// 	return verifier
// }
//
// func createValidCommand(
// 	t *testing.T,
// 	address *ServiceAddress,
// 	authMode iscv1.AuthInfo_AuthMode,
// 	timestamp time.Time,
// ) *iscv1.Command {
// 	t.Helper()
//
// 	// Generate ed25519 key pair directly
// 	privateKeySeed, err := hex.DecodeString(testSignerAddress)
// 	require.NoError(t, err)
// 	privateKey := ed25519.NewKeyFromSeed(privateKeySeed)
// 	publicKey := privateKey.Public().(ed25519.PublicKey)
//
// 	// Create a minimal payload to satisfy validation
// 	payload, err := proto.Marshal(&iscv1.Persona{Id: "test-persona"})
// 	if err != nil {
// 		payload = []byte(`{}`)
// 	}
// 	payloadStruct := &structpb.Struct{
// 		Fields: map[string]*structpb.Value{
// 			"data": {Kind: &structpb.Value_StringValue{StringValue: string(payload)}},
// 		},
// 	}
//
// 	// Generate deterministic salt
// 	salt := make([]byte, 16)
// 	for i := range salt {
// 		salt[i] = byte(i)
// 	}
//
// 	commandRaw := &iscv1.CommandRaw{
// 		Timestamp: timestamppb.New(timestamp),
// 		Salt:      salt,
// 		Body: &iscv1.CommandBody{
// 			Name:    "test-command",
// 			Address: address,
// 			Persona: &iscv1.Persona{Id: "test-persona"},
// 			Payload: payloadStruct,
// 		},
// 	}
//
// 	// Marshal command raw to bytes
// 	commandBytes, err := proto.Marshal(commandRaw)
// 	require.NoError(t, err)
//
// 	// Sign the command bytes directly with ed25519
// 	signature := ed25519.Sign(privateKey, commandBytes)
//
// 	command := &iscv1.Command{
// 		Signature: signature,
// 		AuthInfo: &iscv1.AuthInfo{
// 			Mode:          authMode,
// 			SignerAddress: publicKey,
// 		},
// 		CommandBytes: commandBytes,
// 	}
//
// 	return command
// }
//
// // registryShouldNotBeCalledReplyFunc is a helper function for persona mode tests
// // where the registry should not be called (e.g., when using cached persona).
// func registryShouldNotBeCalledReplyFunc(
// 	t *testing.T,
// 	req *registryv1.QueryPersonaRequest,
// 	tree *iavl.MutableTree,
// ) *registryv1.QueryPersonaResponse {
// 	t.Fatal("Registry should not be called when using cached persona")
// 	return nil
// }
//
// // addPersonaToTree adds a test persona with the test signer to the registry tree and returns
// // the hash, version, and proof needed for registry responses.
// func addPersonaToTree(
// 	t *testing.T,
// 	tree *iavl.MutableTree,
// ) ([]byte, int64, *ics23.CommitmentProof) {
// 	t.Helper()
//
// 	signer, err := sign.NewSigner(testSignerAddress, 42)
// 	require.NoError(t, err)
//
// 	details := PersonaDetails{Signers: [][]byte{signer.GetSignerAddress()}}
// 	detailsBytes, err := details.Marshal()
// 	require.NoError(t, err)
//
// 	_, err = tree.Set([]byte(testPersonaID), detailsBytes)
// 	require.NoError(t, err)
//
// 	hash, version, err := tree.SaveVersion()
// 	require.NoError(t, err)
//
// 	im, err := tree.GetImmutable(version)
// 	require.NoError(t, err)
//
// 	proof, err := im.GetProof([]byte(testPersonaID))
// 	require.NoError(t, err)
//
// 	return hash, version, proof
// }
//
// // buildPersonaResponse builds a standard registry response for persona queries.
// func buildPersonaResponse(t *testing.T, tree *iavl.MutableTree) *registryv1.QueryPersonaResponse {
// 	t.Helper()
//
// 	version, err := tree.GetLatestVersion()
// 	require.NoError(t, err)
//
// 	im, err := tree.GetImmutable(version)
// 	require.NoError(t, err)
//
// 	detailBytes, err := im.Get([]byte(testPersonaID))
// 	require.NoError(t, err)
//
// 	details := PersonaDetails{}
// 	err = details.Unmarshal(detailBytes)
// 	require.NoError(t, err)
//
// 	proof, err := im.GetProof([]byte(testPersonaID))
// 	require.NoError(t, err)
//
// 	return &registryv1.QueryPersonaResponse{
// 		Details:   &registryv1.PersonaDetails{Signers: details.Signers},
// 		ExpiresAt: timestamppb.New(time.Now().Add(15 * time.Minute)),
// 		Version:   version,
// 		Proof:     proof,
// 		Root:      im.Hash(),
// 	}
// }
