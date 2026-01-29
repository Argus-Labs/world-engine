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
		opsMax = 1 << 15 // 32_768 iterations
	)

	// Randomly assign weights for operations at test start.
	enqueueWeight := prng.IntN(100)
	drainWeight := 100 - enqueueWeight // remainder
	queueOps := []int{enqueueWeight, drainWeight}

	impl := command.NewQueue[testutils.SimpleCommand]()
	model := make([]command.Command, 0)

	for range opsMax {
		op := testutils.RandWeightedOp(prng, queueOps)
		switch op {
		case enqueueWeight:

			cmd := testutils.SimpleCommand{Value: int(prng.Int32())}
			payload, err := schema.ToProtoStruct(cmd)
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

		case drainWeight:
			var implResult []command.Command
			impl.Drain(&implResult)

			// Property: drain returns all enqueued commands.
			assert.Equal(t, len(model), len(implResult), "drain count mismatch")

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
	assert.Equal(t, len(model), len(finalResult), "final drain count mismatch")
	for i := range finalResult {
		assert.Equal(t, model[i], finalResult[i], "final command[%d] mismatch", i)
	}
}
