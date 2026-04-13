package cardinal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
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

	bytes, err := schema.Serialize(payload)
	require.NoError(t, err)

	cmdpb := &iscv1.Command{
		Name:    payload.Name(),
		Address: &microv1.ServiceAddress{},
		Persona: &iscv1.Persona{Id: persona},
		Payload: bytes,
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

		assert.Empty(t, collected)
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

// -------------------------------------------------------------------------------------------------
// WithSystemEvent smoke tests
// -------------------------------------------------------------------------------------------------
// WithSystemEventEmitter and WithSystemEventReceiver are just light wrappers over the
// systemEventManager, which is already tested. Here, we just check if the regular system event
// operations work correctly.
// -------------------------------------------------------------------------------------------------

func TestSystem_WithSystemEvent(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSystemEventFixture(t)

		count := prng.IntN(10_000)
		model := make([]testutils.SimpleSystemEvent, count)
		for i := range count {
			model[i] = testutils.SimpleSystemEvent{Value: prng.Int()}
		}

		for _, event := range model {
			fixture.Emitter.Emit(event)
		}

		var results []testutils.SimpleSystemEvent
		for event := range fixture.Receiver.Iter() {
			results = append(results, event)
		}

		assert.Len(t, results, len(model), "completeness: expected %d events, got %d", len(model), len(results))
		for i, result := range results {
			assert.Equal(t, model[i], result, "round-trip integrity: event mismatch at index %d", i)
		}
	})

	t.Run("empty iteration", func(t *testing.T) {
		t.Parallel()

		fixture := newSystemEventFixture(t)

		count := 0
		for range fixture.Receiver.Iter() {
			count++
		}
		assert.Equal(t, 0, count)
	})

	t.Run("early termination", func(t *testing.T) {
		t.Parallel()

		fixture := newSystemEventFixture(t)

		for i := range 10 {
			fixture.Emitter.Emit(testutils.SimpleSystemEvent{Value: i})
		}

		count := 0
		for range fixture.Receiver.Iter() {
			count++
			break
		}
		assert.Equal(t, 1, count)
	})
}

type systemEventFixture struct {
	Emitter  WithSystemEventEmitter[testutils.SimpleSystemEvent]
	Receiver WithSystemEventReceiver[testutils.SimpleSystemEvent]
}

func newSystemEventFixture(t *testing.T) *systemEventFixture {
	t.Helper()

	world := &World{world: ecs.NewWorld()}
	fixture := &systemEventFixture{}

	// We initialize these separately because the default behavior is we don't allow a system to
	// process the same system event type, it doesn't make sense to do it. But here, we want to do it
	// for simplicity, so we have to initialize these manually with different systemEvents sets.
	meta := &systemInitMetadata{world: world, systemEvents: make(map[string]struct{})}
	err := fixture.Emitter.init(meta)
	require.NoError(t, err)

	meta = &systemInitMetadata{world: world, systemEvents: make(map[string]struct{})}
	err = fixture.Receiver.init(meta)
	require.NoError(t, err)

	return fixture
}

// -------------------------------------------------------------------------------------------------
// Search, Contains, Exact, smoke tests
// -------------------------------------------------------------------------------------------------
// The search fields and Ref are just light wrappers over the world state operations, Which is
// already tested. Here, we just check if the regular search operations work. Most of the
// complicated logic in initialization where the cached result is created using reflection. We can
// verify it's working if the operations work correctly.
// -------------------------------------------------------------------------------------------------

func TestSearch_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("iter contains", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		// Create random movers and singles; only movers should appear in Iter.
		var expectedIDs []EntityID
		for range prng.IntN(100) {
			if testutils.RandBool(prng) {
				eid, _ := fixture.Movers.Create()
				expectedIDs = append(expectedIDs, eid)
			} else {
				fixture.Singles.Create()
			}
		}

		var moverIDs []EntityID
		for eid := range fixture.Movers.Iter() {
			moverIDs = append(moverIDs, eid)
		}
		assert.Equal(t, expectedIDs, moverIDs)
	})

	t.Run("iter exact", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		// Create random singles and movers; only singles should appear in Iter.
		var expectedIDs []EntityID
		for range prng.IntN(100) {
			if testutils.RandBool(prng) {
				eid, _ := fixture.Singles.Create()
				expectedIDs = append(expectedIDs, eid)
			} else {
				fixture.Movers.Create()
			}
		}

		var singleIDs []EntityID
		for eid := range fixture.Singles.Iter() {
			singleIDs = append(singleIDs, eid)
		}
		assert.Equal(t, expectedIDs, singleIDs)
	})

	t.Run("get by id", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		compB := testutils.ComponentB{
			ID: prng.Uint64(), Label: testutils.RandString(prng, 8), Enabled: testutils.RandBool(prng)}
		moverID, mover := fixture.Movers.Create()
		mover.A.Set(testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()})
		mover.B.Set(compB)

		singleID, single := fixture.Singles.Create()
		single.A.Set(testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()})

		// Success: correct archetype.
		moverResult, err := fixture.Movers.GetByID(moverID)
		require.NoError(t, err)
		assert.Equal(t, compB, moverResult.B.Get())

		// Wrong archetype.
		_, err = fixture.Movers.GetByID(singleID)
		require.ErrorIs(t, err, ecs.ErrArchetypeMismatch)

		_, err = fixture.Singles.GetByID(moverID)
		require.ErrorIs(t, err, ecs.ErrArchetypeMismatch)

		// Nonexistent entity.
		_, err = fixture.Movers.GetByID(999)
		require.ErrorIs(t, err, ecs.ErrEntityNotFound)
	})

	t.Run("get set remove", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		eid, mover := fixture.Movers.Create()

		// Get returns zero value before Set.
		assert.Equal(t, testutils.ComponentA{}, mover.A.Get())

		// Set then Get round-trips the value.
		compA := testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()}
		mover.A.Set(compA)
		assert.Equal(t, compA, mover.A.Get())

		// Overwrite with a new value.
		compA2 := testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()}
		mover.A.Set(compA2)
		assert.Equal(t, compA2, mover.A.Get())

		// Remove changes the archetype, so the entity no longer matches Movers.
		mover.A.Remove()
		_, err := fixture.Movers.GetByID(eid)
		require.ErrorIs(t, err, ecs.ErrArchetypeMismatch)
	})

	t.Run("destroy", func(t *testing.T) {
		t.Parallel()
		fixture := newSearchFixture(t)

		eid, _ := fixture.Movers.Create()

		// Destroy succeeds once, then fails on the same ID.
		assert.True(t, fixture.Movers.Destroy(eid))
		assert.False(t, fixture.Movers.Destroy(eid))
	})

	t.Run("filter", func(t *testing.T) {
		t.Parallel()
		fixture := newSearchFixture(t)

		eid1, mover1 := fixture.Movers.Create()
		mover1.B.Set(testutils.ComponentB{ID: 1, Label: "one", Enabled: true})

		eid2, mover2 := fixture.Movers.Create()
		mover2.B.Set(testutils.ComponentB{ID: 2, Label: "two", Enabled: false})

		eid3, mover3 := fixture.Movers.Create()
		mover3.B.Set(testutils.ComponentB{ID: 3, Label: "three", Enabled: true})

		allIDs := []EntityID{eid1, eid2, eid3}
		expectedIDs := []EntityID{eid1, eid3}

		var results []EntityID
		for eid := range fixture.Movers.Iter().Filter(func(_ EntityID, mover struct {
			A Ref[testutils.ComponentA]
			B Ref[testutils.ComponentB]
		}) bool {
			return mover.B.Get().Enabled
		}) {
			results = append(results, eid)
		}
		assert.Equal(t, expectedIDs, results)

		var nilPredicateResults []EntityID
		for eid := range fixture.Movers.Iter().Filter(nil) {
			nilPredicateResults = append(nilPredicateResults, eid)
		}
		assert.Equal(t, allIDs, nilPredicateResults)
	})

	t.Run("limit", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		count := prng.IntN(100) + 1
		expectedIDs := make([]EntityID, 0, count)
		for range count {
			eid, _ := fixture.Movers.Create()
			expectedIDs = append(expectedIDs, eid)
		}

		limit := uint32(prng.IntN(count) + 1)
		var results []EntityID
		for eid := range fixture.Movers.Iter().Limit(limit) {
			results = append(results, eid)
		}
		assert.Equal(t, expectedIDs[:limit], results)

		var overLimitResults []EntityID
		for eid := range fixture.Movers.Iter().Limit(uint32(count + 10)) {
			overLimitResults = append(overLimitResults, eid)
		}
		assert.Equal(t, expectedIDs, overLimitResults)
	})

	t.Run("single", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)

		// Exactly one result.
		exactlyOneFixture := newSearchFixture(t)
		eidExpected, mover := exactlyOneFixture.Movers.Create()
		compB := testutils.ComponentB{
			ID:      prng.Uint64(),
			Label:   testutils.RandString(prng, 8),
			Enabled: testutils.RandBool(prng),
		}
		mover.B.Set(compB)

		eid, result, err := exactlyOneFixture.Movers.Iter().Single()
		require.NoError(t, err)
		assert.Equal(t, eidExpected, eid)
		assert.Equal(t, compB, result.B.Get())

		// No results.
		emptyFixture := newSearchFixture(t)
		_, _, err = emptyFixture.Movers.Iter().Single()
		require.ErrorIs(t, err, ErrSingleNoResult)

		// Multiple results.
		multipleFixture := newSearchFixture(t)
		multipleFixture.Movers.Create()
		multipleFixture.Movers.Create()
		_, _, err = multipleFixture.Movers.Iter().Single()
		require.ErrorIs(t, err, ErrSingleMultipleResult)
	})
}

type searchFixture struct {
	// These are the fields under test. They must be public/exported.
	Movers Contains[struct {
		A Ref[testutils.ComponentA]
		B Ref[testutils.ComponentB]
	}]
	Singles Exact[struct {
		A Ref[testutils.ComponentA]
	}]
}

func newSearchFixture(t *testing.T) *searchFixture {
	t.Helper()

	world := &World{world: ecs.NewWorld()}

	fixture := &searchFixture{}

	err := initSystemFields(fixture, world)
	require.NoError(t, err)

	return fixture
}
