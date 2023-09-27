package cmdbuffer_test

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/cmdbuffer"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

func TestLoadingFromRedisShouldNotRepeatEntityIDs(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	var ids []entity.ID
	assert.NilError(t, manager.AtomicFn(func() error {
		var err error
		ids, err = manager.CreateManyEntities(50, fooComp)
		assert.NilError(t, err)
		return nil
	}))

	nextID := ids[len(ids)-1] + 1

	// Make a new manager using the same redis client. Newly assigned ids should start off where
	// the previous manager left off
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	gotID, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, nextID, gotID)
}

func TestComponentSetsCanBeRecovered(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	var firstID entity.ID
	var err error
	assert.NilError(t, manager.AtomicFn(func() error {
		firstID, err = manager.CreateEntity(barComp)
		assert.NilError(t, err)
		return nil
	}))

	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	assert.NilError(t, err)
	assert.NilError(t, manager.AtomicFn(func() error {
		secondID, err := manager.CreateEntity(barComp)
		assert.NilError(t, err)
		firstComps, err := manager.GetComponentTypesForEntity(firstID)
		assert.NilError(t, err)
		secondComps, err := manager.GetComponentTypesForEntity(secondID)
		assert.NilError(t, err)
		assert.Equal(t, len(firstComps), len(secondComps))
		for i := range firstComps {
			assert.Equal(t, firstComps[i].ID(), secondComps[i].ID())
		}
		firstArchID, err := manager.GetArchIDForComponents(firstComps)
		assert.NilError(t, err)
		secondArchID, err := manager.GetArchIDForComponents(secondComps)
		assert.NilError(t, err)
		assert.Equal(t, firstArchID, secondArchID)
		return nil
	}))
}

func getArchIDForEntity(t *testing.T, m *cmdbuffer.Manager, id entity.ID) archetype.ID {
	comps, err := m.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	archID, err := m.GetArchIDForComponents(comps)
	assert.NilError(t, err)
	return archID
}

func TestComponentSetsAreRememberedFromPreviousDB(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	var firstID entity.ID
	var firstArchID archetype.ID
	err := manager.AtomicFn(func() error {
		_, err := manager.CreateEntity(barComp)
		assert.NilError(t, err)
		firstID, err = manager.CreateEntity(fooComp)
		assert.NilError(t, err)
		firstArchID = getArchIDForEntity(t, manager, firstID)
		return nil
	})
	assert.NilError(t, err)
	manager = nil

	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	err = manager.AtomicFn(func() error {
		id, err := manager.CreateEntity(fooComp)
		assert.NilError(t, err)
		gotArchID := getArchIDForEntity(t, manager, id)
		assert.Equal(t, gotArchID, firstArchID)
		return nil
	})
	assert.NilError(t, err)
}

func TestAddedComponentsCanBeDiscarded(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	comps, err := manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
	// Commit this entity creation
	assert.NilError(t, manager.CommitPending())

	assert.NilError(t, manager.AddComponentToEntity(barComp, id))
	comps, err = manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(comps))
	// Discard this added component
	manager.DiscardPending()

	comps, err = manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
}

func TestCanGetComponentTypesAfterReload(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	var id entity.ID
	_, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)

	id, err = manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.CommitPending())

	manager, _ = newCmdBufferAndRedisClientForTest(t, client)

	comps, err := manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(comps))
}

func TestCanDiscardPreviouslyAddedComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)

	err = manager.AtomicFn(func() error {
		assert.NilError(t, manager.AddComponentToEntity(barComp, id))
		// This change will not be accepted
		return errors.New("some error")
	})
	assert.Check(t, err != nil)

	comps, err := manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	// We should only have the foo component
	assert.Equal(t, 1, len(comps))
}

func TestEntitiesCanBeFetchedAfterReload(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	ids, err := manager.CreateManyEntities(10, fooComp, barComp)
	assert.NilError(t, err)
	assert.Equal(t, 10, len(ids))

	comps, err := manager.GetComponentTypesForEntity(ids[0])
	archID, err := manager.GetArchIDForComponents(comps)

	ids = manager.GetEntitiesForArchID(archID)
	assert.Equal(t, 10, len(ids))

	assert.NilError(t, manager.CommitPending())

	// Create a new Manager instances and make sure the previously created entities can be found
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	ids = manager.GetEntitiesForArchID(archID)
	assert.Equal(t, 10, len(ids))
}

func TestTheRemovalOfEntitiesCanBeDiscarded(t *testing.T) {
	manager := newCmdBufferForTest(t)

	ids, err := manager.CreateManyEntities(10, fooComp)
	assert.NilError(t, err)
	comps, err := manager.GetComponentTypesForEntity(ids[0])
	assert.NilError(t, err)
	archID, err := manager.GetArchIDForComponents(comps)
	assert.NilError(t, err)

	gotIDs := manager.GetEntitiesForArchID(archID)
	assert.Equal(t, 10, len(gotIDs))

	err = manager.AtomicFn(func() error {
		// Discard 3 entities
		assert.NilError(t, manager.RemoveEntity(ids[0]))
		assert.NilError(t, manager.RemoveEntity(ids[4]))
		assert.NilError(t, manager.RemoveEntity(ids[7]))

		gotIDs = manager.GetEntitiesForArchID(archID)
		assert.Equal(t, 7, len(gotIDs))

		// Discard these changes (this should bring the entities back)
		return errors.New("some error")
	})
	assert.Check(t, err != nil)

	gotIDs = manager.GetEntitiesForArchID(archID)
	assert.Equal(t, 10, len(gotIDs))
}

func TestTheRemovalOfEntitiesIsRememberedAfterReload(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)

	startingIDs, err := manager.CreateManyEntities(10, fooComp, barComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.CommitPending())

	idToRemove := startingIDs[5]

	err = manager.AtomicFn(func() error {
		assert.NilError(t, manager.RemoveEntity(idToRemove))
		return nil
	})
	assert.NilError(t, err)

	// Start a brand new manager
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	assert.NilError(t, err)

	for _, id := range startingIDs {
		_, err = manager.GetComponentForEntity(fooComp, id)
		if id == idToRemove {
			// Make sure the entity ID we removed cannot be found
			assert.Check(t, err != nil)
		} else {
			assert.NilError(t, err)
		}
	}

}

func TestRemovedComponentDataCanBeRecovered(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	wantFoo := Foo{99}
	assert.NilError(t, manager.SetComponentForEntity(fooComp, id, wantFoo))
	gotFoo, err := manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, wantFoo, gotFoo.(Foo))

	err = manager.AtomicFn(func() error {
		assert.NilError(t, manager.RemoveComponentFromEntity(fooComp, id))

		// Make sure we can no longer get the foo component
		_, err = manager.GetComponentForEntity(fooComp, id)
		assert.ErrorIs(t, err, storage.ErrorComponentNotOnEntity)
		// But uhoh, there was a problem. This means the removal of the Foo component
		// will be undone, and the original value can be found
		return errors.New("some error")
	})
	assert.Check(t, err != nil)

	gotFoo, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, wantFoo, gotFoo.(Foo))
}

func TestArchetypeCountTracksDiscardedChanges(t *testing.T) {
	manager := newCmdBufferForTest(t)

	_, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())

	err = manager.AtomicFn(func() error {
		_, err = manager.CreateEntity(fooComp, barComp)
		assert.NilError(t, err)
		assert.Equal(t, 2, manager.ArchetypeCount())
		return errors.New("some error")
	})
	assert.Check(t, err != nil)

	// The previously created archetype ID was discarded, so the count should be back to 1
	_, err = manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())
}
