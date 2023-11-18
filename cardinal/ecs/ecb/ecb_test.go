package ecb_test

import (
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/cardinaltestutils"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"testing"

	"gotest.tools/v3/assert"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/ecb"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
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
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.RegisterComponents(allComponents))
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
	fooComp       = metadata.NewComponentMetadata[Foo]()
	barComp       = metadata.NewComponentMetadata[Bar]()
	allComponents = []metadata.ComponentMetadata{fooComp, barComp}
)

//nolint:gochecknoinits // its for testing.
func init() {
	_ = fooComp.SetID(1) //notlint:errcheck
	_ = barComp.SetID(2) //notlint:errcheck
}

func TestCanCreateEntityAndSetComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	wantValue := Foo{99}

	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.SetComponentForEntity(fooComp, id, wantValue))
	gotValue, err := manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, wantValue, gotValue)

	// Commit the pending changes
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	// Data should not change after a successful commit
	gotValue, err = manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, wantValue, gotValue)
}

func TestDiscardedComponentChangeRevertsToOriginalValue(t *testing.T) {
	manager := newCmdBufferForTest(t)
	wantValue := Foo{99}

	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.SetComponentForEntity(fooComp, id, wantValue))
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	// Verify the component is what we expect
	gotValue, err := manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, wantValue, gotValue)

	badValue := Foo{666}
	testutils.AssertNilErrorWithTrace(t, manager.SetComponentForEntity(fooComp, id, badValue))
	// The (pending) value should be in the 'bad' state
	gotValue, err = manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, badValue, gotValue)

	// Calling LayerDiscard will discard all changes since the last Layer* call
	manager.DiscardPending()
	// The value should not be the original 'wantValue'
	gotValue, err = manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, wantValue, gotValue)
}

func TestDiscardedEntityIDsWillBeAssignedAgain(t *testing.T) {
	manager := newCmdBufferForTest(t)

	ids, err := manager.CreateManyEntities(10, fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())
	// This is the next ID we should expect to be assigned
	nextID := ids[len(ids)-1] + 1

	// Create a new entity. It should have nextID as the ID
	gotID, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, nextID, gotID)
	// But uhoh, there's a problem. Returning an error here means the entity creation
	// will be undone
	manager.DiscardPending()

	// Create an entity again. We should get nextID assigned again.
	// Create a new entity. It should have nextID as the ID
	gotID, err = manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, nextID, gotID)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	// Now that nextID has been assigned, creating a new entity should give us a new entity ID
	gotID, err = manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, gotID, nextID+1)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())
}

func TestCanGetComponentsForEntity(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)

	comps, err := manager.GetComponentTypesForEntity(id)
	testutils.AssertNilErrorWithTrace(t, err)
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
	testutils.AssertNilErrorWithTrace(t, err)
	comps, err := manager.GetComponentTypesForEntity(firstID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
	// Discard this entity creation
	firstArchID, err := manager.GetArchIDForComponents(comps)
	testutils.AssertNilErrorWithTrace(t, err)

	// Discard the above changes
	manager.DiscardPending()

	// Repeat the above operation. We should end up with the same entity ID, and it should
	// end up containing a different set of components
	gotID, err := manager.CreateEntity(fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	// The assigned entity ID should be reused
	assert.Equal(t, gotID, firstID)
	comps, err = manager.GetComponentTypesForEntity(gotID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 2, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())

	gotArchID, err := manager.GetArchIDForComponents(comps)
	testutils.AssertNilErrorWithTrace(t, err)
	// The archetype ID should be reused
	assert.Equal(t, firstArchID, gotArchID)
}

func TestCannotGetComponentOnEntityThatIsMissingTheComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	// barComp has not been assigned to this entity
	_, err = manager.GetComponentForEntity(barComp, id)
	testutils.AssertErrorIsWithTrace(t, err, storage.ErrComponentNotOnEntity)
}

func TestCannotSetComponentOnEntityThatIsMissingTheComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	// barComp has not been assigned to this entity
	err = manager.SetComponentForEntity(barComp, id, Bar{100})
	testutils.AssertErrorIsWithTrace(t, err, storage.ErrComponentNotOnEntity)
}

func TestCannotRemoveAComponentFromAnEntityThatDoesNotHaveThatComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	err = manager.RemoveComponentFromEntity(barComp, id)
	testutils.AssertErrorIsWithTrace(t, err, storage.ErrComponentNotOnEntity)
}

func TestCanAddAComponentToAnEntity(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	comps, err := manager.GetComponentTypesForEntity(id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
	// Commit this entity creation
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	testutils.AssertNilErrorWithTrace(t, manager.AddComponentToEntity(barComp, id))
	comps, err = manager.GetComponentTypesForEntity(id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 2, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
	assert.Equal(t, comps[1].ID(), barComp.ID())
}

func TestCanRemoveAComponentFromAnEntity(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)

	comps, err := manager.GetComponentTypesForEntity(id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 2, len(comps))

	testutils.AssertNilErrorWithTrace(t, manager.RemoveComponentFromEntity(fooComp, id))
	// Only the barComp should be left
	comps, err = manager.GetComponentTypesForEntity(id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), barComp.ID())
}

func TestCannotAddComponentToEntityThatAlreadyHasTheComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)

	err = manager.AddComponentToEntity(fooComp, id)
	testutils.AssertErrorIsWithTrace(t, err, storage.ErrComponentAlreadyOnEntity)
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

	world := cardinaltestutils.NewTestWorld(t, cardinal.WithStoreManager(manager)).Instance()
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[Health](world))
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[Power](world))
	testutils.AssertNilErrorWithTrace(t, world.LoadGameState())

	wCtx := ecs.NewWorldContext(world)
	justHealthIDs, err := component.CreateMany(wCtx, 8, Health{})
	testutils.AssertNilErrorWithTrace(t, err)
	justPowerIDs, err := component.CreateMany(wCtx, 9, Power{})
	testutils.AssertNilErrorWithTrace(t, err)
	healthAndPowerIDs, err := component.CreateMany(wCtx, 10, Health{}, Power{})
	testutils.AssertNilErrorWithTrace(t, err)

	testCases := []struct {
		filter  ecs.Filterable
		wantIDs []entity.ID
	}{
		{
			filter:  ecs.Contains(Health{}),
			wantIDs: append(justHealthIDs, healthAndPowerIDs...),
		},
		{
			filter:  ecs.Contains(Power{}),
			wantIDs: append(justPowerIDs, healthAndPowerIDs...),
		},
		{
			filter:  ecs.Exact(Health{}, Power{}),
			wantIDs: healthAndPowerIDs,
		},
		{
			filter:  ecs.Exact(Health{}),
			wantIDs: justHealthIDs,
		},
		{
			filter:  ecs.Exact(Power{}),
			wantIDs: justPowerIDs,
		},
	}

	for _, tc := range testCases {
		found := map[entity.ID]bool{}
		var q *ecs.Search
		q, err = world.NewSearch(tc.filter)
		testutils.AssertNilErrorWithTrace(t, err)
		err = q.Each(wCtx, func(id entity.ID) bool {
			found[id] = true
			return true
		})
		testutils.AssertNilErrorWithTrace(t, err)
		assert.Equal(t, len(tc.wantIDs), len(found))
		for _, id := range tc.wantIDs {
			assert.Check(t, found[id], "id is missing from query result")
		}
	}
}

func TestEntityCanBeRemoved(t *testing.T) {
	manager := newCmdBufferForTest(t)

	ids, err := manager.CreateManyEntities(10, fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 10, len(ids))
	for i := range ids {
		if i%2 == 0 {
			testutils.AssertNilErrorWithTrace(t, manager.RemoveEntity(ids[i]))
		}
	}

	comps, err := manager.GetComponentTypesForEntity(ids[1])
	testutils.AssertNilErrorWithTrace(t, err)
	archID, err := manager.GetArchIDForComponents(comps)
	testutils.AssertNilErrorWithTrace(t, err)

	gotIDs, err := manager.GetEntitiesForArchID(archID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 5, len(gotIDs))

	// Only the ids at odd indices should be findable
	for i, id := range ids {
		valid := i%2 == 1
		_, err = manager.GetComponentTypesForEntity(id)
		if valid {
			testutils.AssertNilErrorWithTrace(t, err)
		} else {
			assert.Check(t, err != nil)
		}
	}
}

func TestMovedEntitiesCanBeFoundInNewArchetype(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	startEntityCount := 10
	_, err = manager.CreateManyEntities(startEntityCount, fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)

	fooArchID, err := manager.GetArchIDForComponents([]metadata.ComponentMetadata{fooComp})
	testutils.AssertNilErrorWithTrace(t, err)
	bothArchID, err := manager.GetArchIDForComponents([]metadata.ComponentMetadata{barComp, fooComp})
	testutils.AssertNilErrorWithTrace(t, err)

	// Make sure there are the correct number of ids in each archetype to start
	ids, err := manager.GetEntitiesForArchID(fooArchID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, len(ids))
	ids, err = manager.GetEntitiesForArchID(bothArchID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, startEntityCount, len(ids))

	testutils.AssertNilErrorWithTrace(t, manager.AddComponentToEntity(barComp, id))

	ids, err = manager.GetEntitiesForArchID(fooArchID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 0, len(ids))
	ids, err = manager.GetEntitiesForArchID(bothArchID)
	testutils.AssertNilErrorWithTrace(t, err)
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
	testutils.AssertNilErrorWithTrace(t, manager.RemoveComponentFromEntity(barComp, id))
	ids, err = manager.GetEntitiesForArchID(fooArchID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, len(ids))
	ids, err = manager.GetEntitiesForArchID(bothArchID)
	testutils.AssertNilErrorWithTrace(t, err)
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
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())

	// This archetype has already been created, so it shouldn't change the count
	_, err = manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())

	_, err = manager.CreateEntity(barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 2, manager.ArchetypeCount())

	_, err = manager.CreateEntity(fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 3, manager.ArchetypeCount())
}

func TestClearComponentWhenAnEntityMovesAwayFromAnArchetypeThenBackToTheArchetype(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)

	startValue := Foo{100}

	testutils.AssertNilErrorWithTrace(t, manager.SetComponentForEntity(fooComp, id, startValue))
	gotValue, err := manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, startValue, gotValue.(Foo))

	// Removing fooComp, then re-adding it should zero out the component.
	testutils.AssertNilErrorWithTrace(t, manager.RemoveComponentFromEntity(fooComp, id))
	testutils.AssertNilErrorWithTrace(t, manager.AddComponentToEntity(fooComp, id))

	gotValue, err = manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
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
	testutils.AssertNilErrorWithTrace(t, err)
	idB, err := manager.CreateEntity(barComp, fooComp)
	testutils.AssertNilErrorWithTrace(t, err)

	compsA, err := manager.GetComponentTypesForEntity(idA)
	testutils.AssertNilErrorWithTrace(t, err)
	compsB, err := manager.GetComponentTypesForEntity(idB)
	testutils.AssertNilErrorWithTrace(t, err)

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

	client := redis.NewClient(&options)
	manager, err := ecb.NewManager(client)
	testutils.AssertNilErrorWithTrace(t, err)

	// RegisterComponents must be called before attempting to save the state
	err = manager.CommitPending()
	assert.Check(t, err != nil)

	testutils.AssertNilErrorWithTrace(t, manager.RegisterComponents(allComponents))
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())
}
