package ecs

import (
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing command manager operations
// -------------------------------------------------------------------------------------------------
// This test verifies the commandManager implementation correctness using model-based testing. It
// compares our implementation against a map[string][]micro.Command as the model by applying random
// sequences of receive/clear/get operations to both and asserting equivalence.
// Commands are pre-registered since the micro layer guarantees only registered commands reach ECS.
// -------------------------------------------------------------------------------------------------

func TestCommand_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const numCommands = 10
	const opsMax = 1 << 15 // 32_768 iterations

	impl := newCommandManager()
	model := make(map[string][]micro.Command) // name -> commands buffer

	// Setup: pre-register a fixed set of command names.
	for range numCommands {
		name := randValidCommandName(prng)
		_, err := impl.register(name, reflect.TypeOf(nil))
		require.NoError(t, err)
		model[name] = []micro.Command{}
	}

	for range opsMax {
		op := testutils.RandWeightedOp(prng, commandOps)
		switch op {
		case cr_receive:
			batchSize := prng.IntN(200) + 1
			batch := make([]micro.Command, batchSize)
			for i := range batchSize {
				name := testutils.RandMapKey(prng, model)
				batch[i] = micro.Command{
					Command: micro.CommandRaw{
						Body: micro.CommandBody{Name: name},
					},
				}
			}

			impl.receiveCommands(batch)
			for _, cmd := range batch {
				name := cmd.Command.Body.Name
				model[name] = append(model[name], cmd)
			}

		// NOTE: World calls clear before every tick so commands from previous ticks aren't processed
		// again in the current tick. Here, we call clear randomly to explore edge cases and make sure
		// the implementation is sound even when we're not clearing before every get.
		case cr_clear:
			impl.clear()
			for name := range model {
				model[name] = model[name][:0]
			}

		case cr_get:
			name := testutils.RandMapKey(prng, model)
			implBuf, err := impl.get(name)
			require.NoError(t, err)
			assert.Equal(t, model[name], implBuf, "buffer content mismatch for %q", name)

		default:
			panic("unreachable")
		}
	}

	// Final state check: all buffers match model.
	assert.Len(t, impl.catalog, len(model), "catalog length mismatch")
	for name, modelBuf := range model {
		implBuf, err := impl.get(name)
		require.NoError(t, err, "command %q should be registered", name)
		assert.Len(t, implBuf, len(modelBuf), "buffer length mismatch for %q", name)
	}
}

type commandOp uint8

const (
	cr_receive commandOp = 50
	cr_clear   commandOp = 20
	cr_get     commandOp = 30
)

var commandOps = []commandOp{cr_receive, cr_clear, cr_get}

func randValidCommandName(prng *rand.Rand) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
	length := prng.IntN(50) + 1 // 1-50 characters
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[prng.IntN(len(chars))]
	}
	return string(b)
}

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing command registration
// -------------------------------------------------------------------------------------------------
// This test verifies the commandManager registration correctness using model-based testing. It
// compares our implementation against a map[string]CommandID as the model by applying random
// register operations and asserting equivalence. We also verify structural invariants:
// name-id bijection and ID uniqueness.
// -------------------------------------------------------------------------------------------------

func TestCommand_RegisterModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := newCommandManager()
	model := make(map[string]CommandID) // name -> ID

	for range opsMax {
		name := randValidCommandName(prng)
		implID, err := impl.register(name, reflect.TypeOf(nil))
		require.NoError(t, err)

		if modelID, exists := model[name]; exists {
			assert.Equal(t, modelID, implID, "ID mismatch for re-registered %q", name)
		} else {
			model[name] = implID
		}
	}

	// Property: bijection holds between names and IDs.
	seenIDs := make(map[CommandID]string)
	for name, id := range impl.catalog {
		if prevName, seen := seenIDs[id]; seen {
			t.Errorf("ID %d is mapped by both %q and %q", id, prevName, name)
		}
		seenIDs[id] = name
	}

	// Property: all IDs in catalog are in range [0, nextID).
	for name, id := range impl.catalog {
		assert.Less(t, id, impl.nextID, "ID for %q is out of range", name)
	}

	// Final state check: catalog matches model.
	assert.Len(t, impl.catalog, len(model), "catalog length mismatch")
	for name, modelID := range model {
		implID, exists := impl.catalog[name]
		require.True(t, exists, "command %q should be registered", name)
		assert.Equal(t, modelID, implID, "ID mismatch for %q", name)
	}

	// Simple test to confirm that registering the same name repeatedly is a no-op.
	t.Run("registration idempotence", func(t *testing.T) {
		t.Parallel()

		id1, err := impl.register("hello", reflect.TypeOf(nil))
		require.NoError(t, err)

		id2, err := impl.register("hello", reflect.TypeOf(nil))
		require.NoError(t, err)

		assert.Equal(t, id1, id2)

		id3, err := impl.register("a_different_name", reflect.TypeOf(nil))
		require.NoError(t, err)

		assert.Equal(t, id1+1, id3)
	})
}
