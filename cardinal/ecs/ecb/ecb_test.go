package ecb_test

import (
	"context"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"runtime"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/ecb"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

func newCmdBufferForTest(t *testing.T) *ecb.Manager {
	manager, _ := newCmdBufferAndRedisClientForTest(t, nil)
	return manager
}

// newCmdBufferAndRedisClientForTest creates a ecb.Manager using the given redis client. If the passed in redis
// client is nil, a redis client is created.
func newCmdBufferAndRedisClientForTest(t *testing.T, client *redis.Client) (*ecb.Manager, *redis.Client) {
	if client == nil {
		s := miniredis.RunT(t)
		options := redis.Options{
			Addr:     s.Addr(),
			Password: "", // no password set
			DB:       0,  // use default DB
		}

		client = redis.NewClient(&options)
	}
	manager, err := ecb.NewManager(client)
	assert.NilError(t, err)
	assert.NilError(t, manager.RegisterComponents(allComponents))
	return manager, client
}

type Foo struct {
	Value int
}

func (Foo) Name() string {
	return "foo"
}

type Bar struct {
	Value int
}

func (Bar) Name() string {
	return "bar"
}

var (
	fooComp, errForFooCompGlobal = component.NewComponentMetadata[Foo]()
	barComp, errForBarCompGlobal = component.NewComponentMetadata[Bar]()
	allComponents                = []component.ComponentMetadata{fooComp, barComp}
)

func TestGlobals(t *testing.T) {
	assert.NilError(t, errForFooCompGlobal)
	assert.NilError(t, errForBarCompGlobal)
}

//nolint:gochecknoinits // its for testing.
func init() {
	_ = fooComp.SetID(1) //notlint:errcheck
	_ = barComp.SetID(2) //notlint:errcheck
}

func TestCanCreateEntityAndSetComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()
	wantValue := Foo{99}

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.NilError(t, manager.SetComponentForEntity(fooComp, id, wantValue))
	gotValue, err := manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, wantValue, gotValue)

	// Commit the pending changes
	assert.NilError(t, manager.FinalizeTick(ctx))

	// Data should not change after a successful commit
	gotValue, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, wantValue, gotValue)
}

func TestDiscardedComponentChangeRevertsToOriginalValue(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()
	wantValue := Foo{99}

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.SetComponentForEntity(fooComp, id, wantValue))
	assert.NilError(t, manager.FinalizeTick(ctx))

	// Verify the component is what we expect
	gotValue, err := manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, wantValue, gotValue)

	badValue := Foo{666}
	assert.NilError(t, manager.SetComponentForEntity(fooComp, id, badValue))
	// The (pending) value should be in the 'bad' state
	gotValue, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, badValue, gotValue)

	// Calling LayerDiscard will discard all changes since the last Layer* call
	manager.DiscardPending()
	// The value should not be the original 'wantValue'
	gotValue, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, wantValue, gotValue)
}

func TestDiscardedEntityIDsWillBeAssignedAgain(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	ids, err := manager.CreateManyEntities(10, fooComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.FinalizeTick(ctx))
	// This is the next ID we should expect to be assigned
	nextID := ids[len(ids)-1] + 1

	// Create a new entity. It should have nextID as the ID
	gotID, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, nextID, gotID)
	// But uhoh, there's a problem. Returning an error here means the entity creation
	// will be undone
	manager.DiscardPending()

	// Create an entity again. We should get nextID assigned again.
	// Create a new entity. It should have nextID as the ID
	gotID, err = manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, nextID, gotID)
	assert.NilError(t, manager.FinalizeTick(ctx))

	// Now that nextID has been assigned, creating a new entity should give us a new entity ID
	gotID, err = manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, gotID, nextID+1)
	assert.NilError(t, manager.FinalizeTick(ctx))
}

func TestCanGetComponentsForEntity(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)

	comps, err := manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
}

func TestGettingInvalidEntityResultsInAnError(t *testing.T) {
	manager := newCmdBufferForTest(t)
	_, err := manager.GetComponentTypesForEntity(entity.ID(1034134))
	assert.Check(t, err != nil)
}

func TestComponentSetsCanBeDiscarded(t *testing.T) {
	manager := newCmdBufferForTest(t)

	firstID, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	comps, err := manager.GetComponentTypesForEntity(firstID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
	// Discard this entity creation
	firstArchID, err := manager.GetArchIDForComponents(comps)
	assert.NilError(t, err)

	// Discard the above changes
	manager.DiscardPending()

	// Repeat the above operation. We should end up with the same entity ID, and it should
	// end up containing a different set of components
	gotID, err := manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	// The assigned entity ID should be reused
	assert.Equal(t, gotID, firstID)
	comps, err = manager.GetComponentTypesForEntity(gotID)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())

	gotArchID, err := manager.GetArchIDForComponents(comps)
	assert.NilError(t, err)
	// The archetype ID should be reused
	assert.Equal(t, firstArchID, gotArchID)
}

func TestCannotGetComponentOnEntityThatIsMissingTheComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	// barComp has not been assigned to this entity
	_, err = manager.GetComponentForEntity(barComp, id)
	assert.ErrorIs(t, err, storage.ErrComponentNotOnEntity)
}

func TestCannotSetComponentOnEntityThatIsMissingTheComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	// barComp has not been assigned to this entity
	err = manager.SetComponentForEntity(barComp, id, Bar{100})
	assert.ErrorIs(t, err, storage.ErrComponentNotOnEntity)
}

func TestCannotRemoveAComponentFromAnEntityThatDoesNotHaveThatComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	err = manager.RemoveComponentFromEntity(barComp, id)
	assert.ErrorIs(t, err, storage.ErrComponentNotOnEntity)
}

func TestCanAddAComponentToAnEntity(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	comps, err := manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
	// Commit this entity creation
	assert.NilError(t, manager.FinalizeTick(ctx))

	assert.NilError(t, manager.AddComponentToEntity(barComp, id))
	comps, err = manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
	assert.Equal(t, comps[1].ID(), barComp.ID())
}

func TestCanRemoveAComponentFromAnEntity(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)

	comps, err := manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(comps))

	assert.NilError(t, manager.RemoveComponentFromEntity(fooComp, id))
	// Only the barComp should be left
	comps, err = manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), barComp.ID())
}

func TestCannotAddComponentToEntityThatAlreadyHasTheComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)

	err = manager.AddComponentToEntity(fooComp, id)
	assert.ErrorIs(t, err, storage.ErrComponentAlreadyOnEntity)
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
	manager := newCmdBufferForTest(t)

	engine := testutils.NewTestWorld(t, cardinal.WithStoreManager(manager)).Engine()
	assert.NilError(t, ecs.RegisterComponent[Health](engine))
	assert.NilError(t, ecs.RegisterComponent[Power](engine))
	assert.NilError(t, engine.LoadGameState())

	eCtx := ecs.NewEngineContext(engine)
	justHealthIDs, err := ecs.CreateMany(eCtx, 8, Health{})
	assert.NilError(t, err)
	justPowerIDs, err := ecs.CreateMany(eCtx, 9, Power{})
	assert.NilError(t, err)
	healthAndPowerIDs, err := ecs.CreateMany(eCtx, 10, Health{}, Power{})
	assert.NilError(t, err)

	testCases := []struct {
		filter  filter.ComponentFilter
		wantIDs []entity.ID
	}{
		{
			filter:  filter.Contains(Health{}),
			wantIDs: append(justHealthIDs, healthAndPowerIDs...),
		},
		{
			filter:  filter.Contains(Power{}),
			wantIDs: append(justPowerIDs, healthAndPowerIDs...),
		},
		{
			filter:  filter.Exact(Health{}, Power{}),
			wantIDs: healthAndPowerIDs,
		},
		{
			filter:  filter.Exact(Health{}),
			wantIDs: justHealthIDs,
		},
		{
			filter:  filter.Exact(Power{}),
			wantIDs: justPowerIDs,
		},
	}

	for _, tc := range testCases {
		found := map[entity.ID]bool{}
		q := engine.NewSearch(tc.filter)
		err = q.Each(
			eCtx, func(id entity.ID) bool {
				found[id] = true
				return true
			},
		)
		assert.NilError(t, err)
		assert.Equal(t, len(tc.wantIDs), len(found))
		for _, id := range tc.wantIDs {
			assert.Check(t, found[id], "id is missing from query result")
		}
	}
}

func TestEntityCanBeRemoved(t *testing.T) {
	manager := newCmdBufferForTest(t)

	ids, err := manager.CreateManyEntities(10, fooComp, barComp)
	assert.NilError(t, err)
	assert.Equal(t, 10, len(ids))
	for i := range ids {
		if i%2 == 0 {
			assert.NilError(t, manager.RemoveEntity(ids[i]))
		}
	}

	comps, err := manager.GetComponentTypesForEntity(ids[1])
	assert.NilError(t, err)
	archID, err := manager.GetArchIDForComponents(comps)
	assert.NilError(t, err)

	gotIDs, err := manager.GetEntitiesForArchID(archID)
	assert.NilError(t, err)
	assert.Equal(t, 5, len(gotIDs))

	// Only the ids at odd indices should be findable
	for i, id := range ids {
		valid := i%2 == 1
		_, err = manager.GetComponentTypesForEntity(id)
		if valid {
			assert.NilError(t, err)
		} else {
			assert.Check(t, err != nil)
		}
	}
}

func TestMovedEntitiesCanBeFoundInNewArchetype(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	startEntityCount := 10
	_, err = manager.CreateManyEntities(startEntityCount, fooComp, barComp)
	assert.NilError(t, err)

	fooArchID, err := manager.GetArchIDForComponents([]component.ComponentMetadata{fooComp})
	assert.NilError(t, err)
	bothArchID, err := manager.GetArchIDForComponents([]component.ComponentMetadata{barComp, fooComp})
	assert.NilError(t, err)

	// Make sure there are the correct number of ids in each archetype to start
	ids, err := manager.GetEntitiesForArchID(fooArchID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(ids))
	ids, err = manager.GetEntitiesForArchID(bothArchID)
	assert.NilError(t, err)
	assert.Equal(t, startEntityCount, len(ids))

	assert.NilError(t, manager.AddComponentToEntity(barComp, id))

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
	assert.NilError(t, manager.RemoveComponentFromEntity(barComp, id))
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
	manager := newCmdBufferForTest(t)
	_, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())

	// This archetype has already been created, so it shouldn't change the count
	_, err = manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())

	_, err = manager.CreateEntity(barComp)
	assert.NilError(t, err)
	assert.Equal(t, 2, manager.ArchetypeCount())

	_, err = manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	assert.Equal(t, 3, manager.ArchetypeCount())
}

func TestClearComponentWhenAnEntityMovesAwayFromAnArchetypeThenBackToTheArchetype(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)

	startValue := Foo{100}

	assert.NilError(t, manager.SetComponentForEntity(fooComp, id, startValue))
	gotValue, err := manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, startValue, gotValue.(Foo))

	// Removing fooComp, then re-adding it should zero out the component.
	assert.NilError(t, manager.RemoveComponentFromEntity(fooComp, id))
	assert.NilError(t, manager.AddComponentToEntity(fooComp, id))

	gotValue, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, Foo{}, gotValue.(Foo))
}

func TestCannotCreateEntityWithDuplicateComponents(t *testing.T) {
	manager := newCmdBufferForTest(t)
	_, err := manager.CreateEntity(fooComp, barComp, fooComp)
	assert.Check(t, err != nil)
}

func TestOrderOfComponentsDoesNotMatterWhenCreatingEntities(t *testing.T) {
	manager := newCmdBufferForTest(t)
	idA, err := manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	idB, err := manager.CreateEntity(barComp, fooComp)
	assert.NilError(t, err)

	compsA, err := manager.GetComponentTypesForEntity(idA)
	assert.NilError(t, err)
	compsB, err := manager.GetComponentTypesForEntity(idB)
	assert.NilError(t, err)

	assert.Equal(t, len(compsA), len(compsB))
	for i := range compsA {
		assert.Equal(t, compsA[i].ID(), compsB[i].ID())
	}
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
	manager, err := ecb.NewManager(client)
	assert.NilError(t, err)

	// RegisterComponents must be called before attempting to save the state
	err = manager.FinalizeTick(ctx)
	assert.Check(t, err != nil)

	assert.NilError(t, manager.RegisterComponents(allComponents))
	assert.NilError(t, manager.FinalizeTick(ctx))
}

// TestFinalizeTickPerformanceIsConsistent ensures calls to FinalizeTick takes roughly the same amount of time and
// resources when processing the same amount of data.
func TestFinalizeTickPerformanceIsConsistent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	// createAndFinalizeEntities creates some entities and then calls FinalizeTick. It returns the amount of time it took
	// to execute FinalizeTick and how many bytes of memory were allocated during the call.
	createAndFinalizeEntities := func() (duration time.Duration, allocations uint64) {
		_, err := manager.CreateManyEntities(100, fooComp, barComp)
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
