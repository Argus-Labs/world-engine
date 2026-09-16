package cardinal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type entityTestArchetype struct {
	A testutils.ComponentA
}

type entityTestState struct {
	BaseSystemState
	Entities Query
}

func newEntityTestState(t *testing.T) (*World, *entityTestState) {
	t.Helper()
	w := &World{world: ecs.NewWorld()}
	w.RegisterComponent[testutils.ComponentA]()
	w.RegisterComponent[testutils.ComponentB]()
	s := &entityTestState{BaseSystemState: BaseSystemState{world: w}}
	s.Entities = s.Contains[entityTestArchetype]()
	return w, s
}

func TestEntity_ComponentLifecycle(t *testing.T) {
	t.Parallel()
	_, state := newEntityTestState(t)
	entity := state.Create[entityTestArchetype]()
	require.True(t, entity.Alive())
	assert.Equal(t, testutils.ComponentA{}, entity.Get[testutils.ComponentA]())
	assert.False(t, entity.Has[testutils.ComponentB]())

	entity.Set(testutils.ComponentA{X: 12, Y: 34})
	copyOfA := entity.Get[testutils.ComponentA]()
	copyOfA.X = 99
	assert.Equal(t, testutils.ComponentA{X: 12, Y: 34}, entity.Get[testutils.ComponentA]())
	entity.Set(copyOfA)
	assert.Equal(t, testutils.ComponentA{X: 99, Y: 34}, state.Entity(entity.ID()).Get[testutils.ComponentA]())

	// Optional declarations register components without requiring them in the query.
	entity.Set(testutils.ComponentB{ID: 7, Label: "added", Enabled: true})
	require.True(t, entity.Has[testutils.ComponentB]())
	assert.Equal(t, testutils.ComponentB{ID: 7, Label: "added", Enabled: true}, entity.Get[testutils.ComponentB]())
	entity.Remove[testutils.ComponentB]()
	assert.False(t, entity.Has[testutils.ComponentB]())
	require.Panics(t, func() { entity.Get[testutils.ComponentB]() })
	entity.Remove[testutils.ComponentB]()
	assert.False(t, entity.Has[testutils.ComponentB]())
	assert.False(t, entity.Has[testutils.ComponentC](), "unregistered components are absent")
	require.Panics(t, func() { entity.Set(testutils.ComponentC{}) })

	require.True(t, entity.Destroy())
	assert.False(t, entity.Alive())
	assert.False(t, entity.Has[testutils.ComponentA]())
	assert.False(t, entity.Destroy())
	require.Panics(t, func() { entity.Get[testutils.ComponentA]() })
	require.Panics(t, func() { entity.Set(testutils.ComponentA{}) })
	require.Panics(t, func() { entity.Remove[testutils.ComponentA]() })
}

func TestEntity_WorldIsolationAndSnapshotRestore(t *testing.T) {
	t.Parallel()
	worldA, stateA := newEntityTestState(t)
	_, stateB := newEntityTestState(t)
	a := stateA.Create[entityTestArchetype]()
	b := stateB.Create[entityTestArchetype]()
	require.Equal(t, a.ID(), b.ID(), "numeric IDs are local to a world")
	a.Set(testutils.ComponentA{X: 10})
	b.Set(testutils.ComponentA{X: 20})
	assert.Equal(t, testutils.ComponentA{X: 10}, a.Get[testutils.ComponentA]())
	assert.Equal(t, testutils.ComponentA{X: 20}, b.Get[testutils.ComponentA]())

	// Restore into a newly registered world, then rebind the stored numeric ID.
	restored, restoredState := newEntityTestState(t)
	require.NoError(t, restored.world.FromProto(worldA.world.ToProto()))
	rebound := restoredState.Entity(a.ID())
	require.True(t, rebound.Alive())
	assert.Equal(t, testutils.ComponentA{X: 10}, rebound.Get[testutils.ComponentA]())
	rebound.Set(testutils.ComponentA{X: 30})
	assert.Equal(t, testutils.ComponentA{X: 10}, a.Get[testutils.ComponentA]())
	require.True(t, a.Destroy())
	assert.True(t, rebound.Alive())
	assert.True(t, b.Alive())
}

func TestEntity_QueryHandlesRemainBound(t *testing.T) {
	t.Parallel()
	_, state := newEntityTestState(t)
	first := state.Entities.Create()
	second := state.Entities.Create()
	secondID := second.ID()
	first.Set(testutils.ComponentA{X: 1})
	second.Set(testutils.ComponentA{X: 2})
	var handles []Entity
	for entity := range state.Entities.Iter() {
		// Nested lookups must not change the outer entity's binding.
		other, err := state.Entities.GetByID(secondID)
		require.NoError(t, err)
		assert.Equal(t, testutils.ComponentA{X: 2}, other.Get[testutils.ComponentA]())
		handles = append(handles, entity)
	}
	require.Len(t, handles, 2)
	assert.Equal(t, first, handles[0])
	assert.Equal(t, second, handles[1])
	assert.Equal(t, testutils.ComponentA{X: 1}, handles[0].Get[testutils.ComponentA]())
	assert.Equal(t, testutils.ComponentA{X: 2}, handles[1].Get[testutils.ComponentA]())
	// A copied query has no pointers into another query's result storage.
	queryCopy := state.Entities
	third := queryCopy.Create()
	third.Set(testutils.ComponentA{X: 3})
	assert.Equal(t, testutils.ComponentA{X: 1}, first.Get[testutils.ComponentA]())
	assert.Equal(t, testutils.ComponentA{X: 3}, third.Get[testutils.ComponentA]())
}

func TestEntity_ZeroAndMissingHandles(t *testing.T) {
	t.Parallel()
	_, state := newEntityTestState(t)
	for _, entity := range []Entity{{}, state.Entity(999)} {
		assert.False(t, entity.Alive())
		assert.False(t, entity.Has[testutils.ComponentA]())
		assert.False(t, entity.Destroy())
		require.Panics(t, func() { entity.Get[testutils.ComponentA]() })
		require.Panics(t, func() { entity.Set(testutils.ComponentA{}) })
		require.Panics(t, func() { entity.Remove[testutils.ComponentA]() })
	}
}

func TestEntity_RegisterComponent(t *testing.T) {
	t.Parallel()
	w := &World{world: ecs.NewWorld()}
	state := &BaseSystemState{world: w}
	require.Panics(t, func() { state.Create[entityTestArchetype]() })
	w.RegisterComponent[testutils.ComponentA]()
	w.RegisterComponent[testutils.ComponentA]()
	entity := state.Create[entityTestArchetype]()
	entity.Set(testutils.ComponentA{X: 42})
	assert.Equal(t, testutils.ComponentA{X: 42}, entity.Get[testutils.ComponentA]())
	require.Panics(t, func() { state.Create[int]() })
	require.Panics(t, func() { state.Create[struct{ A int }]() })
	require.Panics(t, func() { state.Create[struct{ A *testutils.ComponentA }]() })
	require.Panics(t, func() { state.Create[struct{ A testutils.ComponentB }]() })
	empty := state.Create[struct{}]()
	assert.True(t, empty.Alive())
	assert.False(t, empty.Has[testutils.ComponentA]())
}

// Queries resolve when built inside a system, so systems may be registered before the
// components they query. An unregistered component fails at the query, not at registration.
func TestQuery_ResolvesAtRunTime(t *testing.T) {
	t.Parallel()
	w := &World{world: ecs.NewWorld()}
	ran := false
	require.NotPanics(t, func() {
		w.RegisterSystem(func(state *entityTestState) {
			ran = true
			require.Panics(t, func() { state.Contains[struct{ B testutils.ComponentB }]() })
			state.Contains[entityTestArchetype]().Create()
			require.Len(t, collect(state.Exact[entityTestArchetype]().Iter()), 1)
		}, WithHook(Init))
	})
	w.RegisterComponent[testutils.ComponentA]()
	w.world.Init()
	require.True(t, ran)
}

func TestQuery_RejectsNonArchetypes(t *testing.T) {
	t.Parallel()
	w := &World{world: ecs.NewWorld()}
	w.RegisterComponent[testutils.ComponentA]()
	state := &BaseSystemState{world: w}
	require.Panics(t, func() { state.Contains[int]() })
	require.Panics(t, func() { state.Exact[struct{ A int }]() })
	require.Panics(t, func() { state.Contains[struct{ A *testutils.ComponentA }]() })
	require.NotPanics(t, func() { state.Contains[struct{ testutils.ComponentA }]() })
	require.NotPanics(t, func() { state.Exact[struct{}]() })
}

func collect(result SearchResult) []Entity {
	var entities []Entity
	for entity := range result {
		entities = append(entities, entity)
	}
	return entities
}

func TestSearch_LimitZeroDoesNotVisitEntities(t *testing.T) {
	t.Parallel()
	_, state := newEntityTestState(t)
	state.Create[entityTestArchetype]()
	visits := 0
	results := state.Entities.Iter().Filter(func(_ Entity) bool {
		visits++
		return true
	}).Limit(0)
	_, err := results.Single()
	require.ErrorIs(t, err, ErrSingleNoResult)
	assert.Zero(t, visits)
}
