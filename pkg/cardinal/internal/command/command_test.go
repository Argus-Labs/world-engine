package command_test

import (
	"math/rand/v2"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing command manager operations
// -------------------------------------------------------------------------------------------------
// This test verifies the command manager implementation correctness by applying random sequences of
// operations and comparing it against a model implementation. Command registration is tested
// separately as it's not part of the "day-to-day" operations of the command manager.
// -------------------------------------------------------------------------------------------------

func TestCommand_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax    = 1 << 15 // 32_768 iterations
		opEnqueue = "enqueue"
		opDrain   = "drain"
		opGet     = "get"
	)

	impl := command.NewManager()
	model := newModelManager()

	// Slice of "generator" helper functions to create typed commands.
	generators := make([]func() command.Payload, 3)
	generators[0] = registerCommand[testutils.CommandA](t, prng, &impl, model)
	generators[1] = registerCommand[testutils.CommandB](t, prng, &impl, model)
	generators[2] = registerCommand[testutils.CommandC](t, prng, &impl, model)

	// Randomize operation weights.
	operations := []string{opEnqueue, opDrain, opGet}
	weights := testutils.RandOpWeights(prng, operations)

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opEnqueue:
			// Pick a random command type and enqueue.
			payload := generators[prng.IntN(len(generators))]()
			pbPayload, err := schema.Serialize(payload)
			require.NoError(t, err)

			persona := testutils.RandString(prng, 8)
			cmdpb := &iscv1.Command{
				Name:    payload.Name(),
				Persona: &iscv1.Persona{Id: persona},
				Payload: pbPayload,
			}

			err = impl.Enqueue(cmdpb)
			require.NoError(t, err)

			model.enqueue(payload.Name(), command.Command{
				Name:    payload.Name(),
				Persona: persona,
				Payload: payload,
			})

		case opDrain:
			modelAll := model.drain()
			implAll := impl.Drain()

			// Property: drain returns all enqueued commands.
			assert.Len(t, implAll, len(modelAll), "drain count mismatch")
			assert.ElementsMatch(t, modelAll, implAll, "drain content mismatch")

			// Property: per-command-type buffers match model via Get.
			for _, gen := range generators {
				name := gen().Name()

				modelBuf, err := model.get(name)
				require.NoError(t, err)

				implBuf, err := impl.Get(model.catalog[name])
				require.NoError(t, err)

				assert.Equal(t, modelBuf, implBuf, "buffer mismatch for command %q", name)
			}

		case opGet:
			// Get for a random command type should match model.
			name := generators[prng.IntN(len(generators))]().Name()

			modelBuf, err := model.get(name)
			require.NoError(t, err)

			implBuf, err := impl.Get(model.catalog[name])
			require.NoError(t, err)

			assert.Equal(t, modelBuf, implBuf, "get mismatch for command %q", name)

		default:
			panic("unreachable")
		}
	}

	// Final state check: drain and verify all buffers match model.
	modelAll := model.drain()
	implAll := impl.Drain()
	assert.Len(t, implAll, len(modelAll), "final drain count mismatch")
	assert.ElementsMatch(t, modelAll, implAll, "final drain content mismatch")

	for _, gen := range generators {
		name := gen().Name()

		modelBuf, err := model.get(name)
		require.NoError(t, err)

		implBuf, err := impl.Get(model.catalog[name])
		require.NoError(t, err)

		assert.Equal(t, modelBuf, implBuf, "final buffer mismatch for command %q", name)
	}
}

func registerCommand[T command.Payload](
	t *testing.T, prng *rand.Rand, impl *command.Manager, model *modelManager,
) func() command.Payload {
	t.Helper()

	var zero T
	name := zero.Name()

	id, err := impl.Register(name, command.NewQueue[T]())
	require.NoError(t, err)

	model.register(id, name)

	switch name {
	case testutils.CommandA{}.Name():
		return func() command.Payload {
			return testutils.CommandA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()}
		}
	case testutils.CommandB{}.Name():
		return func() command.Payload {
			return testutils.CommandB{
				ID:      uint64(prng.IntN(1 << 50)), // Use smaller values to avoid JSON precision loss
				Label:   testutils.RandString(prng, 10),
				Enabled: prng.IntN(2) == 1,
			}
		}
	case testutils.CommandC{}.Name():
		return func() command.Payload {
			return testutils.CommandC{Values: [8]int32{}, Counter: uint16(prng.Int())}
		}
	default:
		panic("unreachable")
	}
}

// modelManager is a simple reference implementation of command.Manager for model-based testing.
// NOTE: The #1 most important aspect of a model is "obvious correctness". The code must be simple
// and obviously correct, no matter the cost on other aspects like performance. Ideally the model
// is small enough to be inlined in the test, but for larger types factoring it out makes the test
// function clearer to read.
type modelManager struct {
	nextID   command.ID
	catalog  map[string]command.ID
	queued   map[string][]command.Command // commands not yet drained
	commands map[string][]command.Command // commands buffer (result of Get)
}

func newModelManager() *modelManager {
	return &modelManager{
		nextID:   0,
		catalog:  make(map[string]command.ID),
		queued:   make(map[string][]command.Command),
		commands: make(map[string][]command.Command),
	}
}

func (m *modelManager) register(id command.ID, name string) {
	m.catalog[name] = id
	m.queued[name] = []command.Command{}
	m.commands[name] = []command.Command{}
}

func (m *modelManager) enqueue(name string, cmd command.Command) error {
	if _, exists := m.catalog[name]; !exists {
		return eris.Errorf("unregistered command: %s", name)
	}
	m.queued[name] = append(m.queued[name], cmd)
	return nil
}

func (m *modelManager) get(name string) ([]command.Command, error) {
	if _, exists := m.catalog[name]; !exists {
		return nil, eris.Errorf("unregistered command: %s", name)
	}
	return m.commands[name], nil
}

func (m *modelManager) drain() []command.Command {
	for name := range m.commands {
		// Move queued  commands to commands buffer and clear queues.
		m.commands[name] = m.queued[name]
		m.queued[name] = []command.Command{}
	}

	var all []command.Command
	for _, cmds := range m.commands {
		all = append(all, cmds...)
	}
	return all
}

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing command registration
// -------------------------------------------------------------------------------------------------
// This test verifies the command manager registration correctness by applying random sequences of
// operations and comparing against a Go map as the model.
// -------------------------------------------------------------------------------------------------

func TestCommand_RegisterModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := command.NewManager()
	model := make(map[string]command.ID) // name -> ID

	for range opsMax {
		name := testutils.RandString(prng, 50)
		implID, err := impl.Register(name, nil)
		require.NoError(t, err)

		if modelID, exists := model[name]; exists {
			// Property: re-registering the same name returns the same ID.
			assert.Equal(t, modelID, implID, "ID mismatch for re-registered %q", name)
		} else {
			model[name] = implID
		}
	}

	// Property: bijection holds between names and IDs (no two names share an ID).
	seenIDs := make(map[command.ID]string)
	for name, id := range model {
		if prevName, seen := seenIDs[id]; seen {
			t.Errorf("ID %d is mapped by both %q and %q", id, prevName, name)
		}
		seenIDs[id] = name
	}

	// Property: all IDs are sequential starting from 0.
	for name, id := range model {
		assert.Less(t, id, command.ID(len(model)), "ID for %q is out of range", name)
	}

	// Property: Get works for all registered commands.
	for name, id := range model {
		buf, err := impl.Get(id)
		require.NoError(t, err, "Get failed for command %q with ID %d", name, id)
		assert.Empty(t, buf, "buffer should be empty for %q", name)
	}
}

// -------------------------------------------------------------------------------------------------
// Concurrent enqueue test
// -------------------------------------------------------------------------------------------------
// This test verifies that concurrent enqueues are thread-safe and all commands are properly stored.
// -------------------------------------------------------------------------------------------------

func TestCommand_ConcurrentEnqueue(t *testing.T) {
	t.Parallel()

	const (
		numGoroutines      = 10
		commandsPerRoutine = 1000
	)

	synctest.Test(t, func(t *testing.T) {
		impl := command.NewManager()

		_, err := impl.Register(testutils.CommandA{}.Name(), command.NewQueue[testutils.CommandA]())
		require.NoError(t, err)
		_, err = impl.Register(testutils.CommandB{}.Name(), command.NewQueue[testutils.CommandB]())
		require.NoError(t, err)

		var wg sync.WaitGroup
		var mu sync.Mutex
		expected := make([]command.Command, 0, numGoroutines*commandsPerRoutine)

		for range numGoroutines {
			wg.Go(func() {
				// Initialize prng in each goroutine separately because rand/v2.Rand isn't concurrent-safe.
				prng := testutils.NewRand(t)

				for i := range commandsPerRoutine {
					var payload command.Payload
					if prng.IntN(2) == 0 {
						payload = testutils.CommandA{X: float64(i), Y: prng.Float64(), Z: 0}
					} else {
						payload = testutils.CommandB{ID: uint64(i), Label: "test", Enabled: true}
					}

					pbPayload, err := schema.Serialize(payload)
					if err != nil {
						t.Errorf("Serialize failed: %v", err)
						return
					}

					cmdpb := &iscv1.Command{
						Name:    payload.Name(),
						Persona: &iscv1.Persona{Id: "test-persona"},
						Payload: pbPayload,
					}

					if err := impl.Enqueue(cmdpb); err != nil {
						t.Errorf("Enqueue failed: %v", err)
						return
					}

					mu.Lock()
					expected = append(expected, command.Command{
						Name:    payload.Name(),
						Persona: "test-persona",
						Payload: payload,
					})
					mu.Unlock()
				}
			})
		}

		// Wait for all goroutines to complete their work.
		wg.Wait()

		// Drain all commands and verify count and content.
		all := impl.Drain()
		expectedTotal := numGoroutines * commandsPerRoutine
		assert.Len(t, all, expectedTotal, "total command count mismatch")
		assert.ElementsMatch(t, expected, all, "command content mismatch")
	})
}
