package command_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	_ "github.com/argus-labs/world-engine/pkg/cardinal/internal/commandtest" // registers fixture codecs
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertCodecRoundTripType marshals value with its registered codec, enqueues it onto a queue typed for
// T, drains it, and asserts the stored payload is exactly T with the same value.
//
// The queue only guards the command name; it stores whatever the codec's Unmarshal returns without
// re-checking its concrete type. The Payload-is-T check lives downstream in newCommandContext as an
// assert.That, which is a no-op in release builds — so a codec that returned the wrong type would, in
// production, silently hand a zero-value T to systems. This test asserts type identity with testify, so
// a mismatched/buggy codec fails the test regardless of build flags.
func assertCodecRoundTripType[T command.Payload](t *testing.T, value T) {
	t.Helper()

	payload, err := command.Marshal(value)
	require.NoError(t, err)

	q := command.NewQueue[T]()
	require.NoError(t, q.Enqueue(&iscv1.Command{
		Name:    value.Name(),
		Address: &microv1.ServiceAddress{},
		Persona: &iscv1.Persona{Id: "round-trip"},
		Payload: payload,
	}))

	var drained []command.Command
	q.Drain(&drained)
	require.Len(t, drained, 1)

	got, ok := drained[0].Payload.(T)
	require.Truef(t, ok, "payload type identity lost: got %T, want %T", drained[0].Payload, value)
	assert.Equal(t, value, got)
}

// TestQueue_CodecRoundTripPreservesType round-trips each fixture command through
// Marshal -> Enqueue -> Drain and asserts the decoded payload keeps its concrete type and value.
func TestQueue_CodecRoundTripPreservesType(t *testing.T) {
	t.Parallel()

	assertCodecRoundTripType(t, testutils.SimpleCommand{Value: 42})
	assertCodecRoundTripType(t, testutils.CommandA{X: 1.5, Y: -2.25, Z: 3.75})
	assertCodecRoundTripType(t, testutils.CommandB{ID: 7, Label: "hello world", Enabled: true})
	assertCodecRoundTripType(t, testutils.CommandC{Values: [8]int32{1, 2, 3, 4, 5, 6, 7, 8}, Counter: 9})
}
