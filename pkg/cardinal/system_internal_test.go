package cardinal

import (
	"context"
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

// -------------------------------------------------------------------------------------------------
// System registration tests
// -------------------------------------------------------------------------------------------------

func TestRegisterSystem_RunsCallerOwnedInstance(t *testing.T) {
	t.Parallel()
	w := &World{world: ecs.NewWorld()}
	count := 7
	system := &privateStateSystem{dependency: &count, scratch: []int{3}}
	w.RegisterSystem(system)

	w.world.Init()
	require.Equal(t, 7, count)
	w.world.Tick()
	w.world.Tick()

	require.Equal(t, 9, count)
	require.Equal(t, []int{3, 8, 9}, system.scratch)
	require.Same(t, w, system.world)
}

// TestRegisterSystem_RunReceivesItsWorld registers one instance in two worlds. Run must read the
// tick, entities, and commands of the world it is handed, not state captured at registration.
func TestRegisterSystem_RunReceivesItsWorld(t *testing.T) {
	t.Parallel()
	first, second := newCommandWorld(t), newCommandWorld(t)
	for _, w := range []*World{first, second} {
		w.RegisterComponent[testutils.ComponentA]()
	}
	system := &worldProbeSystem{}
	first.RegisterSystem(system)
	second.RegisterSystem(system)
	first.currentTick.height = 10
	second.currentTick.height = 20
	first.Create[entityTestArchetype]().Set(testutils.ComponentA{X: 1})
	second.Create[entityTestArchetype]().Set(testutils.ComponentA{X: 2})
	second.Create[entityTestArchetype]().Set(testutils.ComponentA{X: 3})
	enqueueCommand(t, first, testutils.SimpleCommand{Value: 11}, "first")
	enqueueCommand(t, second, testutils.SimpleCommand{Value: 22}, "second")
	first.commands.Drain()
	second.commands.Drain()

	first.world.Init()
	second.world.Init()
	first.world.Tick()
	second.world.Tick()

	require.Len(t, system.worlds, 2)
	require.Same(t, first, system.worlds[0])
	require.Same(t, second, system.worlds[1])
	require.Equal(t, []worldProbeRun{
		{tick: 10, contains: []float64{1}, exact: []float64{1}, commands: []int{11}},
		{tick: 20, contains: []float64{2, 3}, exact: []float64{2, 3}, commands: []int{22}},
	}, system.runs)
}

func TestRegisterSystem_AllowsDistinctInstancesOfSameType(t *testing.T) {
	t.Parallel()
	w := &World{world: ecs.NewWorld()}
	firstCount, secondCount := 10, 20
	w.RegisterSystem(&privateStateSystem{dependency: &firstCount})
	w.RegisterSystem(&privateStateSystem{dependency: &secondCount})

	w.world.Init()
	w.world.Tick()
	require.Equal(t, 11, firstCount)
	require.Equal(t, 21, secondCount)
}

// TestRegisterSystem_RejectsNil: a nil interface or a typed nil pointer would register under a
// valid name and dereference nil inside Run on the first tick, so registration rejects both.
// Value systems are still accepted.
func TestRegisterSystem_RejectsNil(t *testing.T) {
	t.Parallel()
	w := &World{world: ecs.NewWorld()}
	require.Panics(t, func() { w.RegisterSystem(nil) })
	var typedNil *privateStateSystem
	require.Panics(t, func() { w.RegisterSystem(typedNil) })
	require.NotPanics(t, func() { w.RegisterSystem(registeringSystem{}) })
}

func TestRegisterSystem_InitHook(t *testing.T) {
	t.Parallel()
	w := &World{world: ecs.NewWorld()}
	count := 0
	system := &privateStateSystem{dependency: &count}
	w.RegisterSystem(system, WithHook(Init))

	w.world.Init()
	w.world.Tick()
	w.world.Tick()

	require.Equal(t, 1, count)
	require.Equal(t, []int{1}, system.scratch)
}

type worldProbeRun struct {
	tick     uint64
	contains []float64
	exact    []float64
	commands []int
}

type worldProbeSystem struct {
	worlds []*World
	runs   []worldProbeRun
}

func (s *worldProbeSystem) Run(w *World) {
	run := worldProbeRun{tick: w.TickHeight()}
	for entity := range w.Contains[entityTestArchetype]().Iter() {
		run.contains = append(run.contains, entity.Get[testutils.ComponentA]().X)
	}
	for entity := range w.Exact[entityTestArchetype]().Iter() {
		run.exact = append(run.exact, entity.Get[testutils.ComponentA]().X)
	}
	for cmd := range w.Commands[testutils.SimpleCommand]() {
		run.commands = append(run.commands, cmd.Payload.Value)
	}
	s.worlds = append(s.worlds, w)
	s.runs = append(s.runs, run)
}

type privateStateSystem struct {
	dependency *int
	scratch    []int
	world      *World
}

func (s *privateStateSystem) Run(w *World) {
	s.world = w
	(*s.dependency)++
	s.scratch = append(s.scratch, *s.dependency)
}

// -------------------------------------------------------------------------------------------------
// Commands smoke tests
// -------------------------------------------------------------------------------------------------
// Commands is a light wrapper over command.Manager, which is already tested. Here, we just check
// that registration gates the iterator and that the regular command operations work correctly.
// -------------------------------------------------------------------------------------------------

func TestCommands_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		w := newCommandWorld(t)

		count := prng.IntN(100)
		model := make([]testutils.SimpleCommand, count)
		personas := make([]string, count)
		for i := range count {
			model[i] = testutils.SimpleCommand{Value: prng.IntN(1_000_000)} // Bounded to avoid JSON float64 precision loss
			personas[i] = testutils.RandString(prng, 8)
		}

		for i, cmd := range model {
			enqueueCommand(t, w, cmd, personas[i])
		}
		w.commands.Drain()

		var results []CommandContext[testutils.SimpleCommand]
		for ctx := range w.Commands[testutils.SimpleCommand]() {
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

		w := newCommandWorld(t)
		w.commands.Drain()

		count := 0
		for range w.Commands[testutils.SimpleCommand]() {
			count++
		}
		assert.Equal(t, 0, count)
	})

	t.Run("early termination", func(t *testing.T) {
		t.Parallel()

		w := newCommandWorld(t)

		for i := range 10 {
			enqueueCommand(t, w, testutils.SimpleCommand{Value: i}, "player")
		}
		w.commands.Drain()

		count := 0
		for range w.Commands[testutils.SimpleCommand]() {
			count++
			break
		}
		assert.Equal(t, 1, count)
	})

	t.Run("register twice is a no-op", func(t *testing.T) {
		t.Parallel()

		w := newCommandWorld(t)
		require.NotPanics(t, func() { w.RegisterCommand[testutils.SimpleCommand]() })

		enqueueCommand(t, w, testutils.SimpleCommand{Value: 1}, "player")
		w.commands.Drain()

		count := 0
		for range w.Commands[testutils.SimpleCommand]() {
			count++
		}
		assert.Equal(t, 1, count)
	})

	t.Run("unregistered command panics", func(t *testing.T) {
		t.Parallel()

		w := &World{world: ecs.NewWorld(), commands: command.NewManager()}
		require.Panics(t, func() { w.Commands[testutils.SimpleCommand]() })
	})
}

// newCommandWorld returns a world with the command manager and service wired, and SimpleCommand
// registered.
func newCommandWorld(t *testing.T) *World {
	t.Helper()

	w := &World{
		world:    ecs.NewWorld(),
		commands: command.NewManager(),
	}
	w.service = newService(w, AuthModeDev, "")
	w.RegisterCommand[testutils.SimpleCommand]()

	return w
}

// enqueueCommand is a helper that marshals a command payload through its wire layer and enqueues it.
func enqueueCommand(t *testing.T, w *World, payload command.Payload, persona string) {
	t.Helper()

	bytes := schema.Marshal(payload)
	require.NotNil(t, bytes)

	cmdpb := &iscv1.Command{
		Name:    payload.Name(),
		Address: &microv1.ServiceAddress{},
		Persona: &iscv1.Persona{Id: persona},
		Payload: bytes,
	}

	err := w.commands.Enqueue(context.Background(), cmdpb)
	require.NoError(t, err)
}

// -------------------------------------------------------------------------------------------------
// Events smoke tests
// -------------------------------------------------------------------------------------------------
// Broadcast and SendTo are light wrappers over event.Manager, which is already tested. Here, we
// just check that registration gates them and that the regular event operations work correctly.
// -------------------------------------------------------------------------------------------------

func TestEvents_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		w := newEventWorld(t)

		count := prng.IntN(100)
		model := make([]testutils.SimpleEvent, count)
		for i := range count {
			model[i] = testutils.SimpleEvent{Value: prng.Int()}
		}

		for _, evt := range model {
			w.Broadcast(evt)
		}

		collected := dispatchEvents(t, w)
		assert.Len(t, collected, len(model), "completeness: expected %d events, got %d", len(model), len(collected))
		for i, evt := range collected {
			payload, ok := evt.Payload.(testutils.SimpleEvent)
			assert.True(t, ok, "event payload type mismatch at index %d", i)
			assert.Equal(t, model[i], payload, "round-trip integrity: event mismatch at index %d", i)
			assert.Empty(t, evt.Recipient, "broadcast events have no recipient")
		}
	})

	t.Run("send to", func(t *testing.T) {
		t.Parallel()
		w := newEventWorld(t)

		w.SendTo("player", testutils.SimpleEvent{Value: 7})

		collected := dispatchEvents(t, w)
		require.Len(t, collected, 1)
		assert.Equal(t, testutils.SimpleEvent{Value: 7}, collected[0].Payload)
		assert.Equal(t, "player", collected[0].Recipient)
	})

	t.Run("emit empty", func(t *testing.T) {
		t.Parallel()
		w := newEventWorld(t)

		assert.Empty(t, dispatchEvents(t, w))
	})

	t.Run("register twice is a no-op", func(t *testing.T) {
		t.Parallel()
		w := newEventWorld(t)
		require.NotPanics(t, func() { w.RegisterEvent[testutils.SimpleEvent]() })

		w.Broadcast(testutils.SimpleEvent{Value: 1})
		assert.Len(t, dispatchEvents(t, w), 1)
	})

	t.Run("unregistered event panics", func(t *testing.T) {
		t.Parallel()
		w := &World{events: event.NewManager(1024)}
		require.Panics(t, func() { w.Broadcast(testutils.SimpleEvent{Value: 1}) })
		require.Panics(t, func() { w.SendTo("player", testutils.SimpleEvent{Value: 1}) })
	})
}

// newEventWorld returns a world with the event manager wired and SimpleEvent registered.
func newEventWorld(t *testing.T) *World {
	t.Helper()

	w := &World{events: event.NewManager(1024)}
	w.RegisterEvent[testutils.SimpleEvent]()

	return w
}

// dispatchEvents runs the world's default event handlers and returns every event they received.
func dispatchEvents(t *testing.T, w *World) []event.Event {
	t.Helper()

	var collected []event.Event
	w.events.RegisterHandler(event.KindDefault, func(_ context.Context, evt event.Event) error {
		collected = append(collected, evt)
		return nil
	})
	require.NoError(t, w.events.Dispatch(context.Background()))

	return collected
}

// -------------------------------------------------------------------------------------------------
// System events smoke tests
// -------------------------------------------------------------------------------------------------
// EmitSystemEvent and SystemEvents are light wrappers over the ECS system event manager, which is
// already tested. Here, we just check that registration gates them and that the regular system
// event operations work correctly.
// -------------------------------------------------------------------------------------------------

func TestSystemEvents_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		w := newSystemEventWorld(t)

		count := prng.IntN(10_000)
		model := make([]testutils.SimpleSystemEvent, count)
		for i := range count {
			model[i] = testutils.SimpleSystemEvent{Value: prng.Int()}
		}

		for _, event := range model {
			w.EmitSystemEvent(event)
		}

		var results []testutils.SimpleSystemEvent
		for event := range w.SystemEvents[testutils.SimpleSystemEvent]() {
			results = append(results, event)
		}

		assert.Len(t, results, len(model), "completeness: expected %d events, got %d", len(model), len(results))
		for i, result := range results {
			assert.Equal(t, model[i], result, "round-trip integrity: event mismatch at index %d", i)
		}
	})

	t.Run("empty iteration", func(t *testing.T) {
		t.Parallel()

		w := newSystemEventWorld(t)

		count := 0
		for range w.SystemEvents[testutils.SimpleSystemEvent]() {
			count++
		}
		assert.Equal(t, 0, count)
	})

	t.Run("early termination", func(t *testing.T) {
		t.Parallel()

		w := newSystemEventWorld(t)

		for i := range 10 {
			w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: i})
		}

		count := 0
		for range w.SystemEvents[testutils.SimpleSystemEvent]() {
			count++
			break
		}
		assert.Equal(t, 1, count)
	})

	t.Run("register twice is a no-op", func(t *testing.T) {
		t.Parallel()

		w := newSystemEventWorld(t)
		require.NotPanics(t, func() { w.RegisterSystemEvent[testutils.SimpleSystemEvent]() })

		w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: 1})
		count := 0
		for range w.SystemEvents[testutils.SimpleSystemEvent]() {
			count++
		}
		assert.Equal(t, 1, count)
	})

	t.Run("unregistered system event panics", func(t *testing.T) {
		t.Parallel()

		w := &World{world: ecs.NewWorld()}
		require.Panics(t, func() { w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: 1}) })
		require.Panics(t, func() { w.SystemEvents[testutils.SimpleSystemEvent]() })
	})
}

// newSystemEventWorld returns a world with SimpleSystemEvent registered.
func newSystemEventWorld(t *testing.T) *World {
	t.Helper()

	w := &World{world: ecs.NewWorld()}
	w.RegisterSystemEvent[testutils.SimpleSystemEvent]()

	return w
}

// -------------------------------------------------------------------------------------------------
// Search, Contains, Exact, smoke tests
// -------------------------------------------------------------------------------------------------
// Search and Entity are light wrappers over the world state operations, which are already
// tested. Here, we just check if the regular query operations work. The archetype bitmap is
// resolved on the first Contains/Exact call per world and cached; if the operations work, that
// resolution works.
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
				eid := fixture.Movers.Create()
				expectedIDs = append(expectedIDs, eid.ID())
			} else {
				fixture.Singles.Create()
			}
		}

		var moverIDs []EntityID
		for eid := range fixture.Movers.Iter() {
			moverIDs = append(moverIDs, eid.ID())
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
				eid := fixture.Singles.Create()
				expectedIDs = append(expectedIDs, eid.ID())
			} else {
				fixture.Movers.Create()
			}
		}

		var singleIDs []EntityID
		for eid := range fixture.Singles.Iter() {
			singleIDs = append(singleIDs, eid.ID())
		}
		assert.Equal(t, expectedIDs, singleIDs)
	})

	t.Run("get by id", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		compB := testutils.ComponentB{
			ID: prng.Uint64(), Label: testutils.RandString(prng, 8), Enabled: testutils.RandBool(prng)}
		mover := fixture.Movers.Create()
		moverID := mover.ID()
		mover.Set(testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()})
		mover.Set(compB)

		single := fixture.Singles.Create()

		singleID := single.ID()
		single.Set(testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()})

		// Success: correct archetype.
		moverResult, err := fixture.Movers.GetByID(moverID)
		require.NoError(t, err)
		assert.Equal(t, compB, moverResult.Get[testutils.ComponentB]())

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

		mover := fixture.Movers.Create()

		eid := mover.ID()

		// Get returns zero value before Set.
		assert.Equal(t, testutils.ComponentA{}, mover.Get[testutils.ComponentA]())

		// Set then Get round-trips the value.
		compA := testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()}
		mover.Set(compA)
		assert.Equal(t, compA, mover.Get[testutils.ComponentA]())

		// Overwrite with a new value.
		compA2 := testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()}
		mover.Set(compA2)
		assert.Equal(t, compA2, mover.Get[testutils.ComponentA]())

		// Remove changes the archetype, so the entity no longer matches Movers.
		mover.Remove[testutils.ComponentA]()
		_, err := fixture.Movers.GetByID(eid)
		require.ErrorIs(t, err, ecs.ErrArchetypeMismatch)
	})

	t.Run("destroy", func(t *testing.T) {
		t.Parallel()
		fixture := newSearchFixture(t)

		entity := fixture.Movers.Create()

		// Destroy succeeds once, then fails on the same ID.
		assert.True(t, entity.Destroy())
		assert.False(t, entity.Destroy())
	})

	t.Run("filter", func(t *testing.T) {
		t.Parallel()
		fixture := newSearchFixture(t)

		mover1 := fixture.Movers.Create()

		eid1 := mover1.ID()
		mover1.Set(testutils.ComponentB{ID: 1, Label: "one", Enabled: true})

		mover2 := fixture.Movers.Create()

		eid2 := mover2.ID()
		mover2.Set(testutils.ComponentB{ID: 2, Label: "two", Enabled: false})

		mover3 := fixture.Movers.Create()

		eid3 := mover3.ID()
		mover3.Set(testutils.ComponentB{ID: 3, Label: "three", Enabled: true})

		allIDs := []EntityID{eid1, eid2, eid3}
		expectedIDs := []EntityID{eid1, eid3}

		var results []EntityID
		for eid := range fixture.Movers.Iter().Filter(func(mover Entity) bool {
			return mover.Get[testutils.ComponentB]().Enabled
		}) {
			results = append(results, eid.ID())
		}
		assert.Equal(t, expectedIDs, results)

		var nilPredicateResults []EntityID
		for eid := range fixture.Movers.Iter().Filter(nil) {
			nilPredicateResults = append(nilPredicateResults, eid.ID())
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
			eid := fixture.Movers.Create()
			expectedIDs = append(expectedIDs, eid.ID())
		}

		limit := uint32(prng.IntN(count) + 1)
		var results []EntityID
		for eid := range fixture.Movers.Iter().Limit(limit) {
			results = append(results, eid.ID())
		}
		assert.Equal(t, expectedIDs[:limit], results)

		var overLimitResults []EntityID
		for eid := range fixture.Movers.Iter().Limit(uint32(count + 10)) {
			overLimitResults = append(overLimitResults, eid.ID())
		}
		assert.Equal(t, expectedIDs, overLimitResults)
	})

	t.Run("single", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)

		// Exactly one result.
		exactlyOneFixture := newSearchFixture(t)
		mover := exactlyOneFixture.Movers.Create()
		compB := testutils.ComponentB{
			ID:      prng.Uint64(),
			Label:   testutils.RandString(prng, 8),
			Enabled: testutils.RandBool(prng),
		}
		mover.Set(compB)

		result, err := exactlyOneFixture.Movers.Iter().Single()
		require.NoError(t, err)
		assert.Equal(t, mover, result)
		assert.Equal(t, compB, result.Get[testutils.ComponentB]())

		// No results.
		emptyFixture := newSearchFixture(t)
		_, err = emptyFixture.Movers.Iter().Single()
		require.ErrorIs(t, err, ErrSingleNoResult)

		// Multiple results.
		multipleFixture := newSearchFixture(t)
		multipleFixture.Movers.Create()
		multipleFixture.Movers.Create()
		_, err = multipleFixture.Movers.Iter().Single()
		require.ErrorIs(t, err, ErrSingleMultipleResult)
	})
}

type moverArchetype struct {
	A testutils.ComponentA
	B testutils.ComponentB
}

// searchFixture holds the two searches the smoke tests exercise, resolved once against a fresh
// world so each test body can use them like the fields a system used to declare.
type searchFixture struct {
	Movers  Search
	Singles Search
}

func newSearchFixture(t *testing.T) *searchFixture {
	t.Helper()
	w := &World{world: ecs.NewWorld()}
	w.RegisterComponent[testutils.ComponentA]()
	w.RegisterComponent[testutils.ComponentB]()
	return &searchFixture{
		Movers:  w.Contains[moverArchetype](),
		Singles: w.Exact[struct{ A testutils.ComponentA }](),
	}
}
