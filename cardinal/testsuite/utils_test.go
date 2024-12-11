package testsuite

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types"
)

func TestSetTestTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		setup   func(t *testing.T)
	}{
		{
			name:    "sets timeout for test without existing deadline",
			timeout: 100 * time.Millisecond,
		},
		{
			name:    "respects existing deadline",
			timeout: 100 * time.Millisecond,
			setup: func(t *testing.T) {
				t.Parallel()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			SetTestTimeout(t, tt.timeout)
			// Test passes if it doesn't panic
		})
	}
}

func TestUniqueSignatureWithName(t *testing.T) {
	tests := []struct {
		name       string
		personaTag string
	}{
		{
			name:       "generates signature with custom name",
			personaTag: "custom_persona",
		},
		{
			name:       "generates signature with empty name",
			personaTag: "",
		},
		{
			name:       "generates signature with special characters",
			personaTag: "test@123_-.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := UniqueSignatureWithName(tt.personaTag)
			require.NotNil(t, sig)
			assert.Equal(t, tt.personaTag, sig.PersonaTag)
			assert.Equal(t, "namespace", sig.Namespace)
		})
	}
}

func TestUniqueSignature(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "generates unique signatures",
			want: "some_persona_tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig1 := UniqueSignature()
			sig2 := UniqueSignature()

			// Verify signatures are unique but have expected properties
			assert.NotEqual(t, sig1.Hash, sig2.Hash, "signatures should be unique")
			assert.Equal(t, tt.want, sig1.PersonaTag)
			assert.Equal(t, tt.want, sig2.PersonaTag)
			assert.Equal(t, "namespace", sig1.Namespace)
			assert.Equal(t, "namespace", sig2.Namespace)
		})
	}
}

type testInputMsg struct {
	Value string
	id    types.MessageID
}

var (
	// Sentinel errors for test messages
	errTestInputDecode  = errors.New("test input decode error")
	errTestOutputDecode = errors.New("test output decode error")
)

func (t *testInputMsg) SetID(id types.MessageID) error {
	t.id = id
	return nil
}

func (t *testInputMsg) Name() string                          { return "test_input_msg" }
func (t *testInputMsg) Group() string                         { return "test" }
func (t *testInputMsg) FullName() string                      { return "test.test_input_msg" }
func (t *testInputMsg) ID() types.MessageID                   { return t.id }
func (t *testInputMsg) Encode(_ any) ([]byte, error)          { return []byte{}, nil }
func (t *testInputMsg) Decode(_ []byte) (any, error)          { return nil, errTestInputDecode }
func (t *testInputMsg) DecodeEVMBytes(_ []byte) (any, error)  { return nil, errTestInputDecode }
func (t *testInputMsg) ABIEncode(_ any) ([]byte, error)       { return []byte{}, nil }
func (t *testInputMsg) IsEVMCompatible() bool                 { return false }
func (t *testInputMsg) GetInFieldInformation() map[string]any { return map[string]any{} }

type testOutputMsg struct {
	Result bool
	id     types.MessageID
}

func (t *testOutputMsg) SetID(id types.MessageID) error {
	t.id = id
	return nil
}

func (t *testOutputMsg) Name() string                          { return "test_output_msg" }
func (t *testOutputMsg) Group() string                         { return "test" }
func (t *testOutputMsg) FullName() string                      { return "test.test_output_msg" }
func (t *testOutputMsg) ID() types.MessageID                   { return t.id }
func (t *testOutputMsg) Encode(_ any) ([]byte, error)          { return []byte{}, nil }
func (t *testOutputMsg) Decode(_ []byte) (any, error)          { return nil, errTestOutputDecode }
func (t *testOutputMsg) DecodeEVMBytes(_ []byte) (any, error)  { return nil, errTestOutputDecode }
func (t *testOutputMsg) ABIEncode(_ any) ([]byte, error)       { return []byte{}, nil }
func (t *testOutputMsg) IsEVMCompatible() bool                 { return false }
func (t *testOutputMsg) GetInFieldInformation() map[string]any { return map[string]any{} }

func TestGetMessage(t *testing.T) {
	tests := []struct {
		name        string
		msgType     string
		shouldError bool
	}{
		{
			name:        "get registered message",
			msgType:     "test.test_input_msg",
			shouldError: false,
		},
		{
			name:        "get unregistered message",
			msgType:     "test.unregistered_msg",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test world
			world := cardinal.NewTestWorld(t)

			// Register test messages
			err := world.RegisterMessage(&testInputMsg{})
			require.NoError(t, err)
			err = world.RegisterMessage(&testOutputMsg{})
			require.NoError(t, err)

			// Test message retrieval
			_, err = world.GetMessage(tt.msgType)
			if tt.shouldError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
