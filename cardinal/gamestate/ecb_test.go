package gamestate_test

import (
	"bytes"
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/gamestate/search"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	redisstorage "pkg.world.dev/world-engine/cardinal/storage/redis"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/world"
)

type Foo struct{ Value int }

func (Foo) Name() string {
	return "foo"
}

type Bar struct{ Value int }

func (Bar) Name() string {
	return "bar"
}

func newRedisClient(t *testing.T) *redis.Client {
	s := miniredis.RunT(t)
	options := redis.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}
	return redis.NewClient(&options)
}

// newECBForTest world.Creates a gamestate.EntityCommandBuffer using the given
// redis dbStorage. If the passed in redis
// dbStorage is nil, a redis dbStorage is world.Created.
func newECBForTest(t *testing.T, client *redis.Client) *gamestate.EntityCommandBuffer {
	if client == nil {
		client = newRedisClient(t)
	}

	rs := redisstorage.NewRedisStorageWithClient(client, "test")
	state, err := gamestate.New(&rs)
	assert.NilError(t, err)

	fooComp, err := gamestate.NewComponentMetadata[Foo]()
	assert.NilError(t, err)
	barComp, err := gamestate.NewComponentMetadata[Bar]()
	assert.NilError(t, err)

	assert.NilError(t, state.RegisterComponent(fooComp))
	assert.NilError(t, state.RegisterComponent(barComp))

	assert.NilError(t, state.Init())

	return state.ECB()
}

func TestCanCreateEntityAndSetComponent(t *testing.T) {
	ecb := newECBForTest(t, nil)
	ctx := context.Background()
	wantValue := Foo{99}

	id, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	_, err = ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.NilError(t, ecb.SetComponentForEntity(id, wantValue))
	gotValue, err := ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, wantValue, gotValue)

	// Commit the pending changes
	assert.NilError(t, ecb.FinalizeTick(ctx))

	// Data should not change after a successful commit
	gotValue, err = ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, wantValue, gotValue)
}

func TestDiscardedComponentChangeRevertsToOriginalValue(t *testing.T) {
	ecb := newECBForTest(t, nil)
	ctx := context.Background()
	wantValue := Foo{99}

	id, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	assert.NilError(t, ecb.SetComponentForEntity(id, wantValue))
	assert.NilError(t, ecb.FinalizeTick(ctx))

	// Verify the component is what we expect
	gotValue, err := ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, wantValue, gotValue)

	badValue := Foo{666}
	assert.NilError(t, ecb.SetComponentForEntity(id, badValue))
	// The (pending) value should be in the 'bad' state
	gotValue, err = ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, badValue, gotValue)

	// Calling LayerDiscard will discard all changes since the last Layer* call
	err = ecb.DiscardPending()
	assert.NilError(t, err)
	// The value should not be the original 'wantValue'
	gotValue, err = ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, wantValue, gotValue)
}

func TestDiscardedEntityIDsWillBeAssignedAgain(t *testing.T) {
	ecb := newECBForTest(t, nil)
	ctx := context.Background()

	ids, err := ecb.CreateManyEntities(10, Foo{})
	assert.NilError(t, err)
	assert.NilError(t, ecb.FinalizeTick(ctx))
	// This is the next EntityID we should expect to be assigned
	nextID := ids[len(ids)-1] + 1

	// world.Create a new entity. It should have nextID as the EntityID
	gotID, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	assert.Equal(t, nextID, gotID)
	// But uhoh, there's a problem. Returning an error here means the entity creation
	// will be undone
	err = ecb.DiscardPending()
	assert.NilError(t, err)

	// world.Create an entity again. We should get nextID assigned again.
	// world.Create a new entity. It should have nextID as the EntityID
	gotID, err = ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	assert.Equal(t, nextID, gotID)
	assert.NilError(t, ecb.FinalizeTick(ctx))

	// Now that nextID has been assigned, creating a new entity should give us a new entity EntityID
	gotID, err = ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	assert.Equal(t, gotID, nextID+1)
	assert.NilError(t, ecb.FinalizeTick(ctx))
}

func TestCanGetComponentsForEntity(t *testing.T) {
	ecb := newECBForTest(t, nil)
	id, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)

	comp, err := ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, comp.(types.Component).Name(), Foo{}.Name())
}

func TestGettingInvalidEntityResultsInAnError(t *testing.T) {
	ecb := newECBForTest(t, nil)
	_, err := ecb.GetComponentForEntity(Foo{}, types.EntityID(1034134))
	assert.Check(t, err != nil)
}

func TestComponentSetsCanBeDiscarded(t *testing.T) {
	ecb := newECBForTest(t, nil)

	firstID, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	comp, err := ecb.GetComponentForEntity(Foo{}, firstID)
	assert.NilError(t, err)
	assert.Equal(t, comp.(types.Component).Name(), Foo{}.Name())

	// Discard the above changes
	err = ecb.DiscardPending()
	assert.NilError(t, err)

	// Repeat the above operation. We should end up with the same entity EntityID, and it should
	// end up containing a different set of components
	gotID, err := ecb.CreateEntity(Foo{}, Bar{})
	assert.NilError(t, err)

	// The assigned entity EntityID should be reused
	assert.Equal(t, gotID, firstID)
	comps, err := ecb.GetComponentForEntity(Foo{}, gotID)
	assert.NilError(t, err)
	assert.Equal(t, comps.(types.Component).Name(), Foo{}.Name())
}

func TestCannotGetComponentOnEntityThatIsMissingTheComponent(t *testing.T) {
	ecb := newECBForTest(t, nil)
	id, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	// Bar{} has not been assigned to this entity
	_, err = ecb.GetComponentForEntity(Bar{}, id)
	assert.ErrorIs(t, err, gamestate.ErrComponentNotOnEntity)
}

func TestCannotSetComponentOnEntityThatIsMissingTheComponent(t *testing.T) {
	manager := newECBForTest(t, nil)
	id, err := manager.CreateEntity(Foo{})
	assert.NilError(t, err)
	// Bar{} has not been assigned to this entity
	err = manager.SetComponentForEntity(id, Bar{100})
	assert.ErrorIs(t, err, gamestate.ErrComponentNotOnEntity)
}

func TestCannotRemoveAComponentFromAnEntityThatDoesNotHaveThatComponent(t *testing.T) {
	ecb := newECBForTest(t, nil)
	id, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	err = ecb.RemoveComponentFromEntity(Bar{}, id)
	assert.ErrorIs(t, err, gamestate.ErrComponentNotOnEntity)
}

func TestCanAddAComponentToAnEntity(t *testing.T) {
	manager := newECBForTest(t, nil)
	ctx := context.Background()

	// Create an entity with the Foo component
	id, err := manager.CreateEntity(Foo{})
	assert.NilError(t, err)

	// Check that the Foo component is on the entity
	comp, err := manager.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, comp.(types.Component).Name(), Foo{}.Name())

	// Add a Bar component to the entity
	assert.NilError(t, manager.AddComponentToEntity(Bar{}, id))

	// Commit this entity creation
	assert.NilError(t, manager.FinalizeTick(ctx))

	// Check that the Foo component is still on the entity
	comp, err = manager.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, comp.(types.Component).Name(), Foo{}.Name())

	// Check that the Bar component is on the entity
	comp, err = manager.GetComponentForEntity(Bar{}, id)
	assert.NilError(t, err)
	assert.Equal(t, comp.(types.Component).Name(), Bar{}.Name())
}

func TestCanRemoveAComponentFromAnEntity(t *testing.T) {
	manager := newECBForTest(t, nil)

	// Create an entity with both the Foo and Bar components
	id, err := manager.CreateEntity(Foo{}, Bar{})
	assert.NilError(t, err)

	// Check that the Foo component is on the entity
	comp, err := manager.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, comp.(types.Component).Name(), Foo{}.Name())

	// Check that the Bar component is on the entity
	comp, err = manager.GetComponentForEntity(Bar{}, id)
	assert.NilError(t, err)
	assert.Equal(t, comp.(types.Component).Name(), Bar{}.Name())

	// Remove the Foo component from the entity
	assert.NilError(t, manager.RemoveComponentFromEntity(Foo{}, id))

	// Only the Bar component should be left
	comp, err = manager.GetComponentForEntity(Bar{}, id)
	assert.NilError(t, err)
	assert.Equal(t, comp.(types.Component).Name(), Bar{}.Name())
}

func TestCannotAddComponentToEntityThatAlreadyHasTheComponent(t *testing.T) {
	manager := newECBForTest(t, nil)
	id, err := manager.CreateEntity(Foo{})
	assert.NilError(t, err)

	err = manager.AddComponentToEntity(Foo{}, id)
	assert.ErrorIs(t, err, gamestate.ErrComponentAlreadyOnEntity)
}

type Health struct {
	Value int
}

func (Health) Name() string {
	return "health"
}

type Power struct {
	Value int
}

func (Power) Name() string {
	return "power"
}

func TestStorageCanBeUsedInQueries(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[Health](tf.World()))
	assert.NilError(t, world.RegisterComponent[Power](tf.World()))

	var justHealthIDs []types.EntityID
	var justPowerIDs []types.EntityID
	var healthAndPowerIDs []types.EntityID
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		var err error
		justHealthIDs, err = world.CreateMany(wCtx, 8, Health{})
		assert.NilError(t, err)
		justPowerIDs, err = world.CreateMany(wCtx, 9, Power{})
		assert.NilError(t, err)
		healthAndPowerIDs, err = world.CreateMany(wCtx, 10, Health{}, Power{})
		assert.NilError(t, err)
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	testCases := []struct {
		search  func() map[types.EntityID]bool
		wantIDs []types.EntityID
	}{
		{
			search: func() map[types.EntityID]bool {
				found := map[types.EntityID]bool{}
				err := tf.Cardinal.World().Search(filter.Contains(Health{})).Each(
					func(id types.EntityID) bool {
						found[id] = true
						return true
					},
				)
				assert.NilError(t, err)
				return found
			},
			wantIDs: append(justHealthIDs, healthAndPowerIDs...),
		},
		{
			search: func() map[types.EntityID]bool {
				found := map[types.EntityID]bool{}
				err := tf.Cardinal.World().Search(filter.Contains(Power{})).Each(
					func(id types.EntityID) bool {
						found[id] = true
						return true
					},
				)
				assert.NilError(t, err)
				return found
			},
			wantIDs: append(justPowerIDs, healthAndPowerIDs...),
		},
		{
			search: func() map[types.EntityID]bool {
				found := map[types.EntityID]bool{}
				err := tf.Cardinal.World().Search(filter.Exact(Power{}, Health{})).Each(
					func(id types.EntityID) bool {
						found[id] = true
						return true
					},
				)
				assert.NilError(t, err)
				return found
			},
			wantIDs: healthAndPowerIDs,
		},
		{
			search: func() map[types.EntityID]bool {
				found := map[types.EntityID]bool{}
				err := tf.Cardinal.World().Search(filter.Exact(Health{})).Each(
					func(id types.EntityID) bool {
						found[id] = true
						return true
					},
				)
				assert.NilError(t, err)
				return found
			},
			wantIDs: justHealthIDs,
		},
		{
			search: func() map[types.EntityID]bool {
				found := map[types.EntityID]bool{}
				err := tf.Cardinal.World().Search(filter.Exact(Power{})).Each(
					func(id types.EntityID) bool {
						found[id] = true
						return true
					},
				)
				assert.NilError(t, err)
				return found
			},
			wantIDs: justPowerIDs,
		},
	}

	for _, tc := range testCases {
		found := tc.search()
		assert.Equal(t, len(tc.wantIDs), len(found))
		for _, id := range tc.wantIDs {
			assert.Check(t, found[id], "id is missing from query result")
		}
	}
}

func TestEntityCanBeRemoved(t *testing.T) {
	manager := newECBForTest(t, nil)

	ids, err := manager.CreateManyEntities(10, Foo{}, Bar{})
	assert.NilError(t, err)
	assert.Equal(t, 10, len(ids))
	for i := range ids {
		if i%2 == 0 {
			assert.NilError(t, manager.RemoveEntity(ids[i]))
		}
	}

	for i, id := range ids {
		valid := i%2 == 1
		_, err = manager.GetComponentForEntity(Foo{}, id)
		if valid {
			assert.NilError(t, err)
		} else {
			assert.Check(t, err != nil)
		}
	}
}

func TestMovedEntitiesCanBeFoundInNewArchetype(t *testing.T) {
	manager := newECBForTest(t, nil)

	id, err := manager.CreateEntity(Foo{})
	assert.NilError(t, err)

	startEntityCount := 10
	_, err = manager.CreateManyEntities(startEntityCount, Foo{}, Bar{})
	assert.NilError(t, err)

	fooArchIDs, err := manager.FindArchetypes(filter.Exact(Foo{}))
	assert.NilError(t, err)
	fooArchID := fooArchIDs[0]

	bothArchIDs, err := manager.FindArchetypes(filter.Exact(Bar{}, Foo{}))
	assert.NilError(t, err)
	bothArchID := bothArchIDs[0]

	// Make sure there are the correct number of ids in each archetype to start
	ids, err := manager.GetEntitiesForArchID(fooArchID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(ids))

	ids, err = manager.GetEntitiesForArchID(bothArchID)
	assert.NilError(t, err)
	assert.Equal(t, startEntityCount, len(ids))

	assert.NilError(t, manager.AddComponentToEntity(Bar{}, id))

	ids, err = manager.GetEntitiesForArchID(fooArchID)
	assert.NilError(t, err)
	assert.Equal(t, 0, len(ids))

	ids, err = manager.GetEntitiesForArchID(bothArchID)
	assert.NilError(t, err)
	assert.Equal(t, startEntityCount+1, len(ids))

	// make sure the target id is in the new list of ids.
	found := false
	for _, currID := range ids {
		if currID == id {
			found = true
			break
		}
	}
	assert.Check(t, found)

	// Also make sure we can remove the archetype
	assert.NilError(t, manager.RemoveComponentFromEntity(Bar{}, id))

	ids, err = manager.GetEntitiesForArchID(fooArchID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(ids))

	ids, err = manager.GetEntitiesForArchID(bothArchID)
	assert.NilError(t, err)
	assert.Equal(t, startEntityCount, len(ids))

	// Make sure the target id is NOT in the 'both' group
	found = false
	for _, currID := range ids {
		if currID == id {
			found = true
		}
	}
	assert.Check(t, !found)
}

func TestCanGetArchetypeCount(t *testing.T) {
	manager := newECBForTest(t, nil)
	_, err := manager.CreateEntity(Foo{})
	assert.NilError(t, err)
	archCount, err := manager.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 1, archCount)

	// This archetype has already been world.Created, so it shouldn't change the count
	_, err = manager.CreateEntity(Foo{})
	assert.NilError(t, err)
	archCount, err = manager.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 1, archCount)

	_, err = manager.CreateEntity(Bar{})
	assert.NilError(t, err)
	archCount, err = manager.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 2, archCount)

	_, err = manager.CreateEntity(Foo{}, Bar{})
	assert.NilError(t, err)
	archCount, err = manager.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 3, archCount)
}

func TestClearComponentWhenAnEntityMovesAwayFromAnArchetypeThenBackToTheArchetype(t *testing.T) {
	manager := newECBForTest(t, nil)
	id, err := manager.CreateEntity(Foo{}, Bar{})
	assert.NilError(t, err)

	startValue := Foo{100}

	assert.NilError(t, manager.SetComponentForEntity(id, startValue))
	gotValue, err := manager.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, startValue, gotValue.(Foo))

	// Removing Foo{}, then re-adding it should zero out the component.
	assert.NilError(t, manager.RemoveComponentFromEntity(Foo{}, id))
	assert.NilError(t, manager.AddComponentToEntity(Foo{}, id))

	gotValue, err = manager.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, Foo{}, gotValue.(Foo))
}

func TestCannotCreateEntityWithDuplicateComponents(t *testing.T) {
	manager := newECBForTest(t, nil)
	_, err := manager.CreateEntity(Foo{}, Bar{}, Foo{})
	assert.Check(t, err != nil)
}

func TestCannotSaveStateBeforeRegisteringComponents(t *testing.T) {
	// Don't use newCmdBufferForTest because that automatically registers some components.
	s := miniredis.RunT(t)
	options := redis.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}
	ctx := context.Background()

	client := redis.NewClient(&options)
	rs := redisstorage.NewRedisStorageWithClient(client, "")
	state, err := gamestate.New(&rs)
	assert.NilError(t, err)

	// registerComponents must be called before attempting to save the state
	err = state.ECB().FinalizeTick(ctx)
	assert.IsError(t, err)

	fooComp, err := gamestate.NewComponentMetadata[Foo]()
	assert.NilError(t, err)
	barComp, err := gamestate.NewComponentMetadata[Bar]()
	assert.NilError(t, err)

	assert.NilError(t, state.RegisterComponent(fooComp))
	assert.NilError(t, state.RegisterComponent(barComp))

	assert.NilError(t, state.Init())
	assert.NilError(t, state.ECB().FinalizeTick(ctx))
}

// TestFinalizeTickPerformanceIsConsistent ensures calls to FinalizeTick takes roughly the same amount of time and
// resources when processing the same amount of data.
func TestFinalizeTickPerformanceIsConsistent(t *testing.T) {
	manager := newECBForTest(t, nil)
	ctx := context.Background()

	// CreateAndFinalizeEntities world.Creates some entities and then calls FinalizeTick. It returns the amount
	// of time it took to execute FinalizeTick and how many bytes of memory were allocated during the call.
	createAndFinalizeEntities := func() (duration time.Duration, allocations uint64) {
		_, err := manager.CreateManyEntities(100, Foo{}, Bar{})
		assert.NilError(t, err)

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		startAlloc := memStats.TotalAlloc

		startTime := time.Now()
		err = manager.FinalizeTick(ctx)
		deltaTime := time.Since(startTime)

		runtime.ReadMemStats(&memStats)
		deltaAlloc := memStats.TotalAlloc - startAlloc

		// Make sure FinalizeTick didn't produce an error.
		assert.NilError(t, err)
		return deltaTime, deltaAlloc
	}

	// Collect a baseline for how much time FinalizeTick should take and how much memory it should allocate.
	baselineDuration, baselineAlloc := createAndFinalizeEntities()

	// Run FinalizeTick a bunch of times to exacerbate any memory leaks.
	for i := 0; i < 100; i++ {
		_, _ = createAndFinalizeEntities()
	}

	// Run FinalizeTick a final handful of times. We'll take the average of these final runs and compare them to
	// the baseline. Averaging these runs is required to avoid any GC spikes that will cause a single run of
	// FinalizeTick to be slow, or some background process that is allocating memory in bursts.
	var totalDuration time.Duration
	var totalAlloc uint64
	const count = 10
	for i := 0; i < count; i++ {
		currDuration, currAlloc := createAndFinalizeEntities()
		totalDuration += currDuration
		totalAlloc += currAlloc
	}

	averageDuration := totalDuration / count
	averageAlloc := totalAlloc / count

	const maxFactor = 5
	maxDuration := maxFactor * baselineDuration
	maxAlloc := maxFactor * baselineAlloc

	assert.Assert(t, averageDuration < maxDuration,
		"FinalizeTick took an average of %v but must be less than %v", averageDuration, maxDuration)
	assert.Assert(t, averageAlloc < maxAlloc,
		"FinalizeTick allocated an average of %v but must be less than %v", averageAlloc, maxAlloc)
}

func TestLoadingFromRedisShouldNotRepeatEntityIDs(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	ids, err := ecb.CreateManyEntities(50, Foo{})
	assert.NilError(t, err)
	assert.NilError(t, ecb.FinalizeTick(ctx))

	nextID := ids[len(ids)-1] + 1

	// Make a new manager using the same redis dbStorage. Newly assigned ids should start off where
	// the previous manager left off
	ecb = newECBForTest(t, client)
	gotID, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	assert.Equal(t, nextID, gotID)
}

func TestComponentSetsCanBeRecovered(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	firstID, err := ecb.CreateEntity(Bar{})
	assert.NilError(t, err)
	assert.NilError(t, ecb.FinalizeTick(ctx))

	ecb = newECBForTest(t, client)
	assert.NilError(t, err)

	secondID, err := ecb.CreateEntity(Bar{})
	assert.NilError(t, err)

	firstComps, err := ecb.GetAllComponentsForEntityInRawJSON(firstID)
	assert.NilError(t, err)

	secondComps, err := ecb.GetAllComponentsForEntityInRawJSON(secondID)
	assert.NilError(t, err)

	assert.Equal(t, len(firstComps), len(secondComps))
	for compName := range firstComps {
		assert.True(t, bytes.Equal(firstComps[compName], secondComps[compName]))
	}
}

func TestAddedComponentsCanBeDiscarded(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	id, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)

	comps, err := ecb.GetAllComponentsForEntityInRawJSON(id)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(comps))

	// Commit this entity creation
	assert.NilError(t, ecb.FinalizeTick(ctx))

	assert.NilError(t, ecb.AddComponentToEntity(Bar{}, id))
	comps, err = ecb.GetAllComponentsForEntityInRawJSON(id)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(comps))

	// Discard this added component
	err = ecb.DiscardPending()
	assert.NilError(t, err)

	comps, _ = ecb.GetAllComponentsForEntityInRawJSON(id)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(comps))
}

func TestCanGetComponentTypesAfterReload(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	var id types.EntityID
	_, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)

	id, err = ecb.CreateEntity(Foo{}, Bar{})
	assert.NilError(t, err)
	assert.NilError(t, ecb.FinalizeTick(ctx))

	ecb = newECBForTest(t, client)

	comps, err := ecb.GetAllComponentsForEntityInRawJSON(id)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(comps))
}

func TestCanDiscardPreviouslyAddedComponent(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	id, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	assert.NilError(t, ecb.FinalizeTick(ctx))

	assert.NilError(t, ecb.AddComponentToEntity(Bar{}, id))
	err = ecb.DiscardPending()
	assert.NilError(t, err)

	comps, err := ecb.GetAllComponentsForEntityInRawJSON(id)
	assert.NilError(t, err)
	// We should only have the foo component
	assert.Equal(t, 1, len(comps))
}

func TestEntitiesCanBeFetchedAfterReload(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	ids, err := ecb.CreateManyEntities(10, Foo{}, Bar{})
	assert.NilError(t, err)
	assert.Equal(t, 10, len(ids))

	ids, err = search.New(ecb, filter.Exact(Foo{}, Bar{})).Collect()
	assert.NilError(t, err)
	assert.Equal(t, 10, len(ids))

	assert.NilError(t, ecb.FinalizeTick(ctx))

	// Create a new EntityCommandBuffer instances and make sure the previously world.Created entities can be found
	ecb = newECBForTest(t, client)
	ids, err = search.New(ecb, filter.Exact(Foo{}, Bar{})).Collect()
	assert.NilError(t, err)
	assert.Equal(t, 10, len(ids))
}

func TestTheRemovalOfEntitiesCanBeDiscarded(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	ids, err := ecb.CreateManyEntities(10, Foo{})
	assert.NilError(t, err)

	gotIDs, err := search.New(ecb, filter.Exact(Foo{})).Collect()
	assert.NilError(t, err)
	assert.Equal(t, 10, len(gotIDs))

	assert.NilError(t, ecb.FinalizeTick(ctx))

	// Discard 3 entities
	assert.NilError(t, ecb.RemoveEntity(ids[0]))
	assert.NilError(t, ecb.RemoveEntity(ids[4]))
	assert.NilError(t, ecb.RemoveEntity(ids[7]))

	gotIDs, err = search.New(ecb, filter.Exact(Foo{})).Collect()
	assert.NilError(t, err)
	assert.Equal(t, 7, len(gotIDs))

	// Discard these changes (this should bring the entities back)
	err = ecb.DiscardPending()
	assert.NilError(t, err)

	gotIDs, err = search.New(ecb, filter.Exact(Foo{})).Collect()
	assert.NilError(t, err)
	assert.Equal(t, 10, len(gotIDs))
}

func TestTheRemovalOfEntitiesIsRememberedAfterReload(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	startingIDs, err := ecb.CreateManyEntities(10, Foo{}, Bar{})
	assert.NilError(t, err)

	assert.NilError(t, ecb.FinalizeTick(ctx))

	idToRemove := startingIDs[5]

	assert.NilError(t, ecb.RemoveEntity(idToRemove))
	assert.NilError(t, ecb.FinalizeTick(ctx))

	// Start a brand-new manager
	ecb = newECBForTest(t, client)
	assert.NilError(t, err)

	for _, id := range startingIDs {
		_, err = ecb.GetComponentForEntity(Foo{}, id)
		if id == idToRemove {
			// Make sure the entity EntityID we removed cannot be found
			assert.Check(t, err != nil)
		} else {
			assert.NilError(t, err)
		}
	}
}

func TestRemovedComponentDataCanBeRecovered(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	id, err := ecb.CreateEntity(Foo{}, Bar{})
	assert.NilError(t, err)
	wantFoo := Foo{99}
	assert.NilError(t, ecb.SetComponentForEntity(id, wantFoo))
	gotFoo, err := ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, wantFoo, gotFoo.(Foo))

	assert.NilError(t, ecb.FinalizeTick(ctx))

	assert.NilError(t, ecb.RemoveComponentFromEntity(Foo{}, id))

	// Make sure we can no longer get the foo component
	_, err = ecb.GetComponentForEntity(Foo{}, id)
	assert.ErrorIs(t, err, gamestate.ErrComponentNotOnEntity)
	// But uhoh, there was a problem. This means the removal of the Foo component
	// will be undone, and the original value can be found
	err = ecb.DiscardPending()
	assert.NilError(t, err)

	gotFoo, err = ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.Equal(t, wantFoo, gotFoo.(Foo))
}

func TestArchetypeCountTracksDiscardedChanges(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	_, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	archCount, err := ecb.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 1, archCount)
	assert.NilError(t, ecb.FinalizeTick(ctx))

	_, err = ecb.CreateEntity(Foo{}, Bar{})
	assert.NilError(t, err)
	archCount, err = ecb.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 2, archCount)
	err = ecb.DiscardPending()
	assert.NilError(t, err)

	// The previously world.Created archetype EntityID was discarded, so the count should be back to 1
	_, err = ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	archCount, err = ecb.ArchetypeCount()
	assert.NilError(t, err)
}

func TestCannotFetchComponentOnRemovedEntityAfterCommit(t *testing.T) {
	client := newRedisClient(t)
	ecb := newECBForTest(t, client)
	ctx := context.Background()

	id, err := ecb.CreateEntity(Foo{}, Bar{})
	assert.NilError(t, err)
	_, err = ecb.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
	assert.NilError(t, ecb.RemoveEntity(id))

	// The entity has been removed. Trying to get a component for the entity should fail.
	_, err = ecb.GetComponentForEntity(Foo{}, id)
	assert.Check(t, err != nil)

	assert.NilError(t, ecb.FinalizeTick(ctx))

	// Trying to get the same component after committing to the DB should also fail.
	_, err = ecb.GetComponentForEntity(Foo{}, id)
	assert.Check(t, err != nil)
}

func TestArchetypeIDIsConsistentAfterSaveAndLoad(t *testing.T) {
	client := newRedisClient(t)
	state1 := newStateForTest(t, client)
	ecb, _ := state1.ECB(), state1.FinalizedState()

	_, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)

	wantIDs, err := ecb.FindArchetypes(filter.Exact(Foo{}))
	assert.NilError(t, err)
	wantID := wantIDs[0]

	assert.NilError(t, ecb.FinalizeTick(context.Background()))

	// Make a second instance of the engine using the same storage.
	state2 := newStateForTest(t, client)
	ecb, _ = state2.ECB(), state2.FinalizedState()
	gotIDs, err := ecb.FindArchetypes(filter.Exact(Foo{}))
	assert.NilError(t, err)

	gotID := gotIDs[0]

	// Archetype indices should be the same across save/load cycles
	assert.Equal(t, wantID, gotID)
}
