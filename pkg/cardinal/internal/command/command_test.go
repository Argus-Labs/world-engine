package command_test

import (
	"context"
	"math/rand/v2"
	"runtime"
	"sync"
	"testing"
	"weak"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
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
			require.NotNil(t, pbPayload)

			sender := command.PlayerSender(testutils.RandString(prng, 8))
			if testutils.RandBool(prng) {
				sender = command.ShardSender(&microv1.ServiceAddress{ServiceId: testutils.RandString(prng, 8)})
			}

			cmdpb := &iscv1.Command{
				Name:    payload.Name(),
				Address: &microv1.ServiceAddress{},
				Payload: pbPayload,
			}

			err = impl.Enqueue(context.Background(), cmdpb, sender)
			require.NoError(t, err)

			model.enqueue(payload.Name(), command.Command{
				Name:    payload.Name(),
				Address: &microv1.ServiceAddress{},
				Sender:  sender,
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
				Enabled: testutils.RandBool(prng),
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
	model := make(map[string]command.ID)                 // name -> ID
	queue := command.NewQueue[testutils.SimpleCommand]() // Only its payload type is read here.

	for range opsMax {
		name := testutils.RandString(prng, 50)
		implID, err := impl.Register(name, queue)
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

// Names feeds DST's command ops, so its order must not depend on map iteration.
func TestCommand_NamesSorted(t *testing.T) {
	t.Parallel()

	m := command.NewManager()

	queue := command.NewQueue[testutils.SimpleCommand]()
	for _, name := range []string{"c", "a", "b"} {
		_, err := m.Register(name, queue)
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"a", "b", "c"}, m.Names())
}

// impostorCommand is a different payload type that shares SimpleCommand's name.
type impostorCommand struct{ Value int }

func (impostorCommand) Name() string { return testutils.SimpleCommand{}.Name() }

func TestCommand_RegisterRejectsNameCollision(t *testing.T) {
	t.Parallel()

	m := command.NewManager()
	name := testutils.SimpleCommand{}.Name()
	id, err := m.Register(name, command.NewQueue[testutils.SimpleCommand]())
	require.NoError(t, err)

	again, err := m.Register(name, command.NewQueue[testutils.SimpleCommand]())
	require.NoError(t, err)
	assert.Equal(t, id, again, "registering the same type again returns its ID")

	_, err = m.Register(name, command.NewQueue[impostorCommand]())
	require.ErrorContains(t, err, "command simple_command already registered with a different type")
	assert.Equal(t, testutils.SimpleCommand{}, m.Zero(name), "the name keeps its registered type")
}

// -------------------------------------------------------------------------------------------------
// Concurrent enqueue test
// -------------------------------------------------------------------------------------------------
// This test verifies that concurrent enqueues are thread-safe and all commands are properly stored.
// Run with -race to detect data races (go test -race). We coordinate goroutine start with an
// explicit barrier to maximize contention in the Enqueue path.
// -------------------------------------------------------------------------------------------------

func TestCommand_ConcurrentEnqueue(t *testing.T) {
	t.Parallel()

	const (
		numGoroutines      = 10
		commandsPerRoutine = 1000
	)

	impl := command.NewManager()

	_, err := impl.Register(testutils.CommandA{}.Name(), command.NewQueue[testutils.CommandA]())
	require.NoError(t, err)
	_, err = impl.Register(testutils.CommandB{}.Name(), command.NewQueue[testutils.CommandB]())
	require.NoError(t, err)

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	start := make(chan struct{})
	expected := make([]command.Command, 0, numGoroutines*commandsPerRoutine)

	for range numGoroutines {
		wg.Go(func() {
			// Initialize prng in each goroutine separately because rand/v2.Rand isn't concurrent-safe.
			prng := testutils.NewRand(t)

			<-start

			for i := range commandsPerRoutine {
				var payload command.Payload
				if testutils.RandBool(prng) {
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
					Address: &microv1.ServiceAddress{},
					Payload: pbPayload,
				}

				if err := impl.Enqueue(context.Background(), cmdpb, command.PlayerSender("test-player")); err != nil {
					t.Errorf("Enqueue failed: %v", err)
					return
				}

				mu.Lock()

				expected = append(expected, command.Command{
					Name:    payload.Name(),
					Address: &microv1.ServiceAddress{},
					Sender:  command.PlayerSender("test-player"),
					Payload: payload,
				})
				mu.Unlock()
			}
		})
	}

	// Release all workers simultaneously to maximize overlapping Enqueue calls.
	close(start)

	// Wait for all goroutines to complete their work.
	wg.Wait()

	// Drain all commands and verify count and content.
	all := impl.Drain()
	expectedTotal := numGoroutines * commandsPerRoutine
	assert.Len(t, all, expectedTotal, "total command count mismatch")
	assert.ElementsMatch(t, expected, all, "command content mismatch")
}

// -------------------------------------------------------------------------------------------------
// Payload retention
// -------------------------------------------------------------------------------------------------
// A long-lived shard reuses its command buffers every tick. These tests verify that reuse doesn't
// keep processed payloads reachable: once commands have been drained and the next tick (or Clear)
// retires them, the GC must be able to free their decoded payloads.
// -------------------------------------------------------------------------------------------------

type retentionCommand struct{ Data []byte }

func (retentionCommand) Name() string { return "retention_command" }

func TestCommand_RetiredPayloadsAreCollectable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		retire func(*command.Manager)
	}{
		{name: "next drain", retire: func(m *command.Manager) { m.Drain() }},
		{name: "clear", retire: func(m *command.Manager) { m.Clear() }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, refs := drainRetentionCommands(t, tc.retire)
			assert.Equal(t, 0, liveRefs(refs), "manager retains retired command payloads")
			runtime.KeepAlive(m)

			// Control: m is dead past KeepAlive, so with the manager gone nothing else may hold the
			// payloads. A failure above can then only come from the manager's buffers.
			assert.Equal(t, 0, liveRefs(refs), "payloads retained outside the manager")
		})
	}
}

// drainRetentionCommands enqueues and drains a batch of commands, records weak pointers to their
// decoded payloads, then retires them. It isn't inlined so no payload stays live on the test stack.
//
//go:noinline
func drainRetentionCommands(
	t *testing.T, retire func(*command.Manager),
) (*command.Manager, []weak.Pointer[byte]) {
	t.Helper()

	const n = 16

	m := command.NewManager()
	id, err := m.Register(retentionCommand{}.Name(), command.NewQueue[retentionCommand]())
	require.NoError(t, err)

	for range n {
		payload, err := schema.Serialize(retentionCommand{Data: make([]byte, 4<<10)})
		require.NoError(t, err)

		err = m.Enqueue(t.Context(), &iscv1.Command{
			Name:    retentionCommand{}.Name(),
			Address: &microv1.ServiceAddress{},
			Payload: payload,
		}, command.PlayerSender("player"))
		require.NoError(t, err)
	}

	require.Len(t, m.Drain(), n)

	commands, err := m.Get(id)
	require.NoError(t, err)

	refs := make([]weak.Pointer[byte], 0, n)

	for _, cmd := range commands {
		payload, ok := cmd.Payload.(retentionCommand)
		require.True(t, ok)

		refs = append(refs, weak.Make(&payload.Data[0]))
	}

	retire(&m)

	return &m, refs
}

// liveRefs runs two full GCs and counts the weak pointers whose targets are still reachable. A
// single GC can keep a dead payload for one cycle when it scans an asynchronously preempted frame
// conservatively; only a real retainer survives the second.
//
//go:noinline
func liveRefs(refs []weak.Pointer[byte]) int {
	runtime.GC()
	runtime.GC()

	live := 0

	for _, ref := range refs {
		if ref.Value() != nil {
			live++
		}
	}

	return live
}
