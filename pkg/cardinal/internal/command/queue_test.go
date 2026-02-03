package command_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing queue operations
// -------------------------------------------------------------------------------------------------
// This test verifies the queue implementation correctness by applying random sequences of queue
// operations and comparing it against a regular Go slice as the model.
// -------------------------------------------------------------------------------------------------

func TestQueue_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax    = 1 << 15 // 32_768 iterations
		opEnqueue = "enqueue"
		opDrain   = "drain"
	)

	// Randomize operation weights.
	operations := []string{opEnqueue, opDrain}
	weights := testutils.RandOpWeights(prng, operations)

	impl := command.NewQueue[testutils.SimpleCommand]()
	model := make([]command.Command, 0)

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opEnqueue:

			cmd := testutils.SimpleCommand{Value: int(prng.Int32())}
			payload, err := schema.ToMsgpack(cmd)
			require.NoError(t, err)

			name := cmd.Name()
			corruptName := prng.IntN(10) == 1 // 10% chance to corrupt the command name.
			if corruptName {
				name = "wrong-name"
			}
			persona := "value doesn't matter"

			cmdpb := &iscv1.Command{
				Name:    name,
				Persona: &iscv1.Persona{Id: persona},
				Payload: payload,
			}

			sizeBefore := impl.Len()
			err = impl.Enqueue(cmdpb)

			if corruptName {
				// Property: enqueue with wrong name must fail.
				require.Error(t, err, "enqueue should fail for mismatched command name")
				// Property: queue size unchanged after failed enqueue.
				assert.Equal(t, sizeBefore, impl.Len(), "queue size should not change on error")
			} else {
				require.NoError(t, err, "enqueue should succeed for valid command")
				model = append(model, command.Command{
					Name:    name,
					Persona: persona,
					Payload: cmd,
				})
			}

		case opDrain:
			var implResult []command.Command
			impl.Drain(&implResult)

			// Property: drain returns all enqueued commands.
			assert.Len(t, implResult, len(model), "drain count mismatch")

			// Property: FIFO ordering preserved.
			for i := range implResult {
				assert.Equal(t, model[i], implResult[i], "command[%d] mismatch", i)
			}

			// Property: queue is empty after drain.
			assert.Zero(t, impl.Len(), "queue should be empty after drain")

			// Property: second drain yields nothing (idempotent).
			var secondDrain []command.Command
			impl.Drain(&secondDrain)
			assert.Empty(t, secondDrain, "second drain should yield no new commands")

			// Clear model.
			model = model[:0]

		default:
			panic("unreachable")
		}
	}

	// Final state check: drain remaining and verify equivalence.
	var finalResult []command.Command
	impl.Drain(&finalResult)
	assert.Len(t, finalResult, len(model), "final drain count mismatch")
	for i := range finalResult {
		assert.Equal(t, model[i], finalResult[i], "final command[%d] mismatch", i)
	}
}

// -------------------------------------------------------------------------------------------------
// uint64 precision test
// -------------------------------------------------------------------------------------------------
// This test verifies that MessagePack serialization preserves uint64 precision for command
// payloads with values above 2^53-1, which would be corrupted by JSON's float64 representation.
// -------------------------------------------------------------------------------------------------

func TestQueue_Uint64Precision(t *testing.T) {
	t.Parallel()

	// Test values that would lose precision with JSON (values > 2^53-1 = 9007199254740991)
	testCases := []testutils.CommandUint64{
		{
			Amount:    18446744073709551615, // uint64 max
			EntityID:  9007199254740993,     // 2^53 + 1, loses precision in JSON
			Timestamp: 9223372036854775807,  // int64 max
		},
		{
			Amount:    10000000000000000000, // 10^19
			EntityID:  9007199254740992,     // 2^53, first value that loses precision
			Timestamp: -9223372036854775808, // int64 min
		},
	}

	queue := command.NewQueue[testutils.CommandUint64]()

	// Enqueue commands with large uint64 values
	for _, tc := range testCases {
		payload, err := schema.ToMsgpack(tc)
		require.NoError(t, err)

		cmdpb := &iscv1.Command{
			Name:    tc.Name(),
			Persona: &iscv1.Persona{Id: "test-persona"},
			Payload: payload,
		}

		err = queue.Enqueue(cmdpb)
		require.NoError(t, err)
	}

	// Drain and verify precision is preserved
	var result []command.Command
	queue.Drain(&result)

	require.Len(t, result, len(testCases))
	for i, expected := range testCases {
		actual, ok := result[i].Payload.(testutils.CommandUint64)
		require.True(t, ok, "payload type mismatch at %d", i)

		assert.Equal(t, expected.Amount, actual.Amount,
			"Amount mismatch at %d: expected %d, got %d (would fail with JSON)", i, expected.Amount, actual.Amount)
		assert.Equal(t, expected.EntityID, actual.EntityID,
			"EntityID mismatch at %d: expected %d, got %d (would fail with JSON)", i, expected.EntityID, actual.EntityID)
		assert.Equal(t, expected.Timestamp, actual.Timestamp,
			"Timestamp mismatch at %d: expected %d, got %d", i, expected.Timestamp, actual.Timestamp)
	}
}
