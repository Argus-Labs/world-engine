package command_test

import (
	"context"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertCodecRoundTripType sends MessagePack through a typed queue and checks its concrete payload.
func assertCodecRoundTripType[T command.Payload](t *testing.T, value T) {
	t.Helper()

	payload, err := schema.Serialize(value)
	require.NoError(t, err)

	q := command.NewQueue[T]()
	require.NoError(t, q.Enqueue(context.Background(), &iscv1.Command{
		Name:    value.Name(),
		Address: &microv1.ServiceAddress{},
		Payload: payload,
	}, command.PlayerSender("round-trip")))

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
	assertCodecRoundTripType(t, plainCommand{ID: 9007199254740993, Values: []int{-7, 31}})
}

type plainCommand struct {
	ID     uint64
	Values []int
}

func (plainCommand) Name() string { return "plain" }

func TestQueueRejectsMalformedPayload(t *testing.T) {
	t.Parallel()
	q := command.NewQueue[plainCommand]()
	// BIN8 instead of an array: the decoder must return an error rather than panic or enqueue.
	err := q.Enqueue(context.Background(), &iscv1.Command{
		Name:    "plain",
		Payload: []byte{0x81, 0xa6, 'V', 'a', 'l', 'u', 'e', 's', 0xc4, 0x01, 'x'},
	}, command.PlayerSender("player"))
	require.ErrorContains(t, err, "failed to decode command payload")
	require.Zero(t, q.Len())
}
