package cardinal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: test system registration, e.g. duplicate field detection, etc.

// -------------------------------------------------------------------------------------------------
// WithCommand smoke tests
// -------------------------------------------------------------------------------------------------
// WithCommand is a light wrapper over command.Manager, which is already tested. Here, we just check
// if the regular command operations work correctly.
// -------------------------------------------------------------------------------------------------

func TestWithCommand_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newCommandFixture(t)

		count := prng.IntN(100)
		model := make([]testutils.SimpleCommand, count)
		personas := make([]string, count)
		for i := range count {
			model[i] = testutils.SimpleCommand{Value: prng.IntN(1_000_000)} // Bounded to avoid JSON float64 precision loss
			personas[i] = testutils.RandString(prng, 8)
		}

		for i, cmd := range model {
			fixture.enqueueCommand(t, cmd, personas[i])
		}
		fixture.world.commands.Drain()

		var results []CommandContext[testutils.SimpleCommand]
		for ctx := range fixture.Command.Iter() {
			results = append(results, ctx)
		}

		assert.Len(t, results, len(model), "completeness: expected %d commands, got %d", len(model), len(results))
		for i, result := range results {
			assert.Equal(t, model[i], result.Payload, "round-trip integrity: payload mismatch at index %d", i)
			assert.Equal(t, personas[i], result.Persona, "round-trip integrity: persona mismatch at index %d", i)
		}
	})

	t.Run("empty iteration", func(t *testing.T) {
		t.Parallel()

		fixture := newCommandFixture(t)
		fixture.world.commands.Drain()

		count := 0
		for range fixture.Command.Iter() {
			count++
		}
		assert.Equal(t, 0, count)
	})

	t.Run("early termination", func(t *testing.T) {
		t.Parallel()

		fixture := newCommandFixture(t)

		for i := range 10 {
			fixture.enqueueCommand(t, testutils.SimpleCommand{Value: i}, "player")
		}
		fixture.world.commands.Drain()

		count := 0
		for range fixture.Command.Iter() {
			count++
			break
		}
		assert.Equal(t, 1, count)
	})
}

type commandFixture struct {
	world   *World
	Command WithCommand[testutils.SimpleCommand]
}

func newCommandFixture(t *testing.T) *commandFixture {
	t.Helper()

	world := &World{
		commands: command.NewManager(),
		service:  newService(nil),
	}

	fixture := &commandFixture{world: world}

	meta := &systemInitMetadata{world: world, commands: make(map[string]struct{}), events: make(map[string]struct{})}
	err := fixture.Command.init(meta)
	require.NoError(t, err)

	return fixture
}

// enqueueCommand is a helper that marshals a command payload to protobuf and enqueues it.
func (f *commandFixture) enqueueCommand(t *testing.T, payload command.Payload, persona string) {
	t.Helper()

	// TODO: refactor to serializable after merge
	pbPayload, err := schema.ToProtoStruct(payload)
	require.NoError(t, err)

	cmdpb := &iscv1.Command{
		Name:    payload.Name(),
		Persona: &iscv1.Persona{Id: persona},
		Payload: pbPayload,
	}

	err = f.world.commands.Enqueue(cmdpb)
	require.NoError(t, err)
}

// -------------------------------------------------------------------------------------------------
// WithEvent smoke tests
// -------------------------------------------------------------------------------------------------
// WithEvent is a light wrapper over event.Manager, which is already tested. Here, we just check if
// the regular event operations work correctly.
// -------------------------------------------------------------------------------------------------

func TestWithEvent_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newEventFixture(t)

		count := prng.IntN(100)
		model := make([]testutils.SimpleEvent, count)
		for i := range count {
			model[i] = testutils.SimpleEvent{Value: prng.Int()}
		}

		for _, evt := range model {
			fixture.Event.Emit(evt)
		}

		// Dispatch collects events and calls registered handlers.
		var collected []event.Event
		fixture.world.events.RegisterHandler(event.KindDefault, func(evt event.Event) error {
			collected = append(collected, evt)
			return nil
		})
		err := fixture.world.events.Dispatch()
		require.NoError(t, err)

		assert.Len(t, collected, len(model), "completeness: expected %d events, got %d", len(model), len(collected))
		for i, evt := range collected {
			payload, ok := evt.Payload.(testutils.SimpleEvent)
			assert.True(t, ok, "event payload type mismatch at index %d", i)
			assert.Equal(t, model[i], payload, "round-trip integrity: event mismatch at index %d", i)
		}
	})

	t.Run("emit empty", func(t *testing.T) {
		t.Parallel()
		fixture := newEventFixture(t)

		var collected []event.Event
		fixture.world.events.RegisterHandler(event.KindDefault, func(evt event.Event) error {
			collected = append(collected, evt)
			return nil
		})
		err := fixture.world.events.Dispatch()
		require.NoError(t, err)

		assert.Len(t, collected, 0)
	})
}

type eventFixture struct {
	world *World
	Event WithEvent[testutils.SimpleEvent]
}

func newEventFixture(t *testing.T) *eventFixture {
	t.Helper()

	world := &World{
		events: event.NewManager(1024),
	}

	fixture := &eventFixture{world: world}

	meta := &systemInitMetadata{world: world, commands: make(map[string]struct{}), events: make(map[string]struct{})}
	err := fixture.Event.init(meta)
	require.NoError(t, err)

	return fixture
}
