package ecb_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/ecb"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestLoadingFromRedisShouldNotRepeatEntityIDs(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	ids, err := manager.CreateManyEntities(50, fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	nextID := ids[len(ids)-1] + 1

	// Make a new manager using the same redis client. Newly assigned ids should start off where
	// the previous manager left off
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	gotID, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, nextID, gotID)
}

func TestComponentSetsCanBeRecovered(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	firstID, err := manager.CreateEntity(barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	testutils.AssertNilErrorWithTrace(t, err)

	secondID, err := manager.CreateEntity(barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	firstComps, err := manager.GetComponentTypesForEntity(firstID)
	testutils.AssertNilErrorWithTrace(t, err)
	secondComps, err := manager.GetComponentTypesForEntity(secondID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, len(firstComps), len(secondComps))
	for i := range firstComps {
		assert.Equal(t, firstComps[i].ID(), secondComps[i].ID())
	}
	firstArchID, err := manager.GetArchIDForComponents(firstComps)
	testutils.AssertNilErrorWithTrace(t, err)
	secondArchID, err := manager.GetArchIDForComponents(secondComps)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, firstArchID, secondArchID)
}

func getArchIDForEntity(t *testing.T, m *ecb.Manager, id entity.ID) archetype.ID {
	comps, err := m.GetComponentTypesForEntity(id)
	testutils.AssertNilErrorWithTrace(t, err)
	archID, err := m.GetArchIDForComponents(comps)
	testutils.AssertNilErrorWithTrace(t, err)
	return archID
}

func TestComponentSetsAreRememberedFromPreviousDB(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	_, err := manager.CreateEntity(barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	firstID, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	firstArchID := getArchIDForEntity(t, manager, firstID)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	testutils.AssertNilErrorWithTrace(t, err)

	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	gotArchID := getArchIDForEntity(t, manager, id)
	assert.Equal(t, gotArchID, firstArchID)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())
}

func TestAddedComponentsCanBeDiscarded(t *testing.T) {
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
	// Discard this added component
	manager.DiscardPending()

	comps, err = manager.GetComponentTypesForEntity(id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
}

func TestCanGetComponentTypesAfterReload(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	var id entity.ID
	_, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)

	id, err = manager.CreateEntity(fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	manager, _ = newCmdBufferAndRedisClientForTest(t, client)

	comps, err := manager.GetComponentTypesForEntity(id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 2, len(comps))
}

func TestCanDiscardPreviouslyAddedComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	testutils.AssertNilErrorWithTrace(t, manager.AddComponentToEntity(barComp, id))
	manager.DiscardPending()

	comps, err := manager.GetComponentTypesForEntity(id)
	testutils.AssertNilErrorWithTrace(t, err)
	// We should only have the foo component
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
}

func TestEntitiesCanBeFetchedAfterReload(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	ids, err := manager.CreateManyEntities(10, fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 10, len(ids))

	comps, err := manager.GetComponentTypesForEntity(ids[0])
	testutils.AssertNilErrorWithTrace(t, err)
	archID, err := manager.GetArchIDForComponents(comps)
	testutils.AssertNilErrorWithTrace(t, err)

	ids, err = manager.GetEntitiesForArchID(archID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 10, len(ids))

	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	// Create a new Manager instances and make sure the previously created entities can be found
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	ids, err = manager.GetEntitiesForArchID(archID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 10, len(ids))
}

func TestTheRemovalOfEntitiesCanBeDiscarded(t *testing.T) {
	manager := newCmdBufferForTest(t)

	ids, err := manager.CreateManyEntities(10, fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	comps, err := manager.GetComponentTypesForEntity(ids[0])
	testutils.AssertNilErrorWithTrace(t, err)
	archID, err := manager.GetArchIDForComponents(comps)
	testutils.AssertNilErrorWithTrace(t, err)

	gotIDs, err := manager.GetEntitiesForArchID(archID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 10, len(gotIDs))
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	// Discard 3 entities
	testutils.AssertNilErrorWithTrace(t, manager.RemoveEntity(ids[0]))
	testutils.AssertNilErrorWithTrace(t, manager.RemoveEntity(ids[4]))
	testutils.AssertNilErrorWithTrace(t, manager.RemoveEntity(ids[7]))

	gotIDs, err = manager.GetEntitiesForArchID(archID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 7, len(gotIDs))

	// Discard these changes (this should bring the entities back)
	manager.DiscardPending()

	gotIDs, err = manager.GetEntitiesForArchID(archID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 10, len(gotIDs))
}

func TestTheRemovalOfEntitiesIsRememberedAfterReload(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	startingIDs, err := manager.CreateManyEntities(10, fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	idToRemove := startingIDs[5]

	testutils.AssertNilErrorWithTrace(t, manager.RemoveEntity(idToRemove))
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	// Start a brand-new manager
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	testutils.AssertNilErrorWithTrace(t, err)

	for _, id := range startingIDs {
		_, err = manager.GetComponentForEntity(fooComp, id)
		if id == idToRemove {
			// Make sure the entity ID we removed cannot be found
			assert.Check(t, err != nil)
		} else {
			testutils.AssertNilErrorWithTrace(t, err)
		}
	}
}

func TestRemovedComponentDataCanBeRecovered(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	wantFoo := Foo{99}
	testutils.AssertNilErrorWithTrace(t, manager.SetComponentForEntity(fooComp, id, wantFoo))
	gotFoo, err := manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, wantFoo, gotFoo.(Foo))

	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	testutils.AssertNilErrorWithTrace(t, manager.RemoveComponentFromEntity(fooComp, id))

	// Make sure we can no longer get the foo component
	_, err = manager.GetComponentForEntity(fooComp, id)
	testutils.AssertErrorIsWithTrace(t, err, storage.ErrComponentNotOnEntity)
	// But uhoh, there was a problem. This means the removal of the Foo component
	// will be undone, and the original value can be found
	manager.DiscardPending()

	gotFoo, err = manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, wantFoo, gotFoo.(Foo))
}

func TestArchetypeCountTracksDiscardedChanges(t *testing.T) {
	manager := newCmdBufferForTest(t)

	_, err := manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	_, err = manager.CreateEntity(fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 2, manager.ArchetypeCount())
	manager.DiscardPending()

	// The previously created archetype ID was discarded, so the count should be back to 1
	_, err = manager.CreateEntity(fooComp)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())
}

func TestCannotFetchComponentOnRemovedEntityAfterCommit(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp, barComp)
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = manager.GetComponentForEntity(fooComp, id)
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, manager.RemoveEntity(id))

	// The entity has been removed. Trying to get a component for the entity should fail.
	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.Check(t, err != nil)

	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	// Trying to get the same component after committing to the DB should also fail.
	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.Check(t, err != nil)
}
