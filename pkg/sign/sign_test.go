package sign_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/sign"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestSigner_NewSigner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		privateKey  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid key",
			privateKey: "0000000000000000000000000000000000000000000000000000000000000000",
			wantErr:    false,
		},
		{
			name:        "invalid hex",
			privateKey:  "not-a-hex-string",
			wantErr:     true,
			errContains: "failed to decode hex private key",
		},
		{
			name:        "wrong key length",
			privateKey:  "0000",
			wantErr:     true,
			errContains: "private key must be 32 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			signer, err := sign.NewSigner(tt.privateKey, 12345)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, signer)
		})
	}
}

func TestSigner_SignCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command *iscv1.CommandBody
		wantErr bool
	}{
		{
			name: "successful signing",
			command: &iscv1.CommandBody{
				Name:    "test-command",
				Payload: createTestPayload(t, 1),
				Persona: &iscv1.Persona{Id: "test-persona"},
				Address: micro.GetAddress("test-region", micro.RealmWorld, "test-org", "test-proj", "test-service"),
			},
			wantErr: false,
		},
		// Can't really force proto.Marshal to return an error, so we'll just test the happy path.
	}

	// Generate a known key pair for testing
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(privateKey.Seed())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			signer, err := sign.NewSigner(privateKeyHex, 12345)
			require.NoError(t, err)

			signedCommand, err := signer.SignCommand(tt.command, iscv1.AuthInfo_AUTH_MODE_PERSONA)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, signedCommand)

			// Verify signature manually
			assert.True(t, ed25519.Verify(publicKey, signedCommand.GetCommandBytes(), signedCommand.GetSignature()))
		})
	}
}

func createTestPayload(t *testing.T, seed int64) *structpb.Struct {
	t.Helper()

	r := rand.New(rand.NewSource(seed))

	// Generate random fields of different types
	fields := map[string]any{
		"string_field":  fmt.Sprintf("value-%d", r.Int31()),
		"integer_field": r.Int63(),
		"float_field":   r.Float64(),
		"bool_field":    r.Int31()%2 == 0,
		"array_field":   []any{r.Int31(), fmt.Sprintf("item-%d", r.Int31()), r.Float64()},
		"nested_field": map[string]any{
			"nested_string": fmt.Sprintf("nested-%d", r.Int31()),
			"nested_int":    r.Int31(),
		},
	}

	eventBody, err := structpb.NewStruct(fields)
	require.NoError(t, err)
	return eventBody
}

func TestVerifyCommandSignature(t *testing.T) {
	t.Parallel()

	// Generate a valid key pair for testing
	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	privateKeyHex := hex.EncodeToString(privateKey.Seed())

	// Create a test command
	command := &iscv1.CommandBody{
		Name:    "test-command",
		Payload: createTestPayload(t, 1),
		Persona: &iscv1.Persona{Id: "test-persona"},
		Address: micro.GetAddress("test-region", micro.RealmWorld, "test-org", "test-proj", "test-service"),
	}

	// Create a signer to sign the command
	signer, err := sign.NewSigner(privateKeyHex, 12345)
	require.NoError(t, err)

	tests := []struct {
		name         string
		setupCommand func(*iscv1.Command) *iscv1.Command
		wantIsValid  bool
	}{
		{
			name: "valid command signature",
			setupCommand: func(sc *iscv1.Command) *iscv1.Command {
				return sc // return unmodified
			},
			wantIsValid: true,
		},
		{
			name: "invalid signature - modified signature",
			setupCommand: func(sc *iscv1.Command) *iscv1.Command {
				// Alter the signature to invalidate it
				if len(sc.GetSignature()) > 0 {
					sc.Signature[0] ^= 1
				}
				return sc
			},
			wantIsValid: false,
		},
		{
			name: "invalid signature - modified command",
			setupCommand: func(sc *iscv1.Command) *iscv1.Command {
				// Alter the command to invalidate the signature
				if len(sc.GetCommandBytes()) > 0 {
					sc.CommandBytes[0] ^= 1
				}
				return sc
			},
			wantIsValid: false,
		},
		{
			name: "invalid signature - incorrect signer address",
			setupCommand: func(sc *iscv1.Command) *iscv1.Command {
				// Generate a different key to use as invalid signer
				wrongPub, _, _ := ed25519.GenerateKey(nil)
				sc.AuthInfo.SignerAddress = wrongPub
				return sc
			},
			wantIsValid: false,
		},
		{
			name: "empty signature",
			setupCommand: func(sc *iscv1.Command) *iscv1.Command {
				sc.Signature = []byte{}
				return sc
			},
			wantIsValid: false,
		},
		{
			name: "nil command",
			setupCommand: func(sc *iscv1.Command) *iscv1.Command {
				sc.CommandBytes = nil
				return sc
			},
			wantIsValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Sign the command for each test case
			signedCommand, err := signer.SignCommand(command, iscv1.AuthInfo_AUTH_MODE_PERSONA)
			require.NoError(t, err)

			// Apply test-specific modifications to the signed command
			signedCommand = tt.setupCommand(signedCommand)

			// Verify the command
			isValid := sign.VerifyCommandSignature(signedCommand)
			assert.Equal(t, tt.wantIsValid, isValid, "VerifyCommandSignature returned unexpected result")
		})
	}
}
