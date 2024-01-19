package gamestate_test

import (
	"context"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

func TestLoadingFromRedisShouldNotRepeatEntityIDs(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)
	ctx := context.Background()

	ids, err := manager.CreateManyEntities(50, fooComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.FinalizeTick(ctx))

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
	ctx := context.Background()

	firstID, err := manager.CreateEntity(barComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.FinalizeTick(ctx))

	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	assert.NilError(t, err)

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
}

func getArchIDForEntity(t *testing.T, m *gamestate.EntityComponentBuffer, id entity.ID) archetype.ID {
	comps, err := m.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	archID, err := m.GetArchIDForComponents(comps)
	assert.NilError(t, err)
	return archID
}

func TestComponentSetsAreRememberedFromPreviousDB(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)
	ctx := context.Background()

	_, err := manager.CreateEntity(barComp)
	assert.NilError(t, err)
	firstID, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	firstArchID := getArchIDForEntity(t, manager, firstID)
	assert.NilError(t, manager.FinalizeTick(ctx))

	assert.NilError(t, err)

	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	gotArchID := getArchIDForEntity(t, manager, id)
	assert.Equal(t, gotArchID, firstArchID)
	assert.NilError(t, manager.FinalizeTick(ctx))
}

func TestAddedComponentsCanBeDiscarded(t *testing.T) {
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
	// Discard this added component
	manager.DiscardPending()

	comps, err = manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
}

func TestCanGetComponentTypesAfterReload(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)
	ctx := context.Background()

	var id entity.ID
	_, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)

	id, err = manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.FinalizeTick(ctx))

	manager, _ = newCmdBufferAndRedisClientForTest(t, client)

	comps, err := manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(comps))
}

func TestCanDiscardPreviouslyAddedComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.FinalizeTick(ctx))

	assert.NilError(t, manager.AddComponentToEntity(barComp, id))
	manager.DiscardPending()

	comps, err := manager.GetComponentTypesForEntity(id)
	assert.NilError(t, err)
	// We should only have the foo component
	assert.Equal(t, 1, len(comps))
	assert.Equal(t, comps[0].ID(), fooComp.ID())
}

func TestEntitiesCanBeFetchedAfterReload(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)
	ctx := context.Background()

	ids, err := manager.CreateManyEntities(10, fooComp, barComp)
	assert.NilError(t, err)
	assert.Equal(t, 10, len(ids))

	comps, err := manager.GetComponentTypesForEntity(ids[0])
	assert.NilError(t, err)
	archID, err := manager.GetArchIDForComponents(comps)
	assert.NilError(t, err)

	ids, err = manager.GetEntitiesForArchID(archID)
	assert.NilError(t, err)
	assert.Equal(t, 10, len(ids))

	assert.NilError(t, manager.FinalizeTick(ctx))

	// Create a new EntityComponentBuffer instances and make sure the previously created entities can be found
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	ids, err = manager.GetEntitiesForArchID(archID)
	assert.NilError(t, err)
	assert.Equal(t, 10, len(ids))
}

func TestTheRemovalOfEntitiesCanBeDiscarded(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	ids, err := manager.CreateManyEntities(10, fooComp)
	assert.NilError(t, err)
	comps, err := manager.GetComponentTypesForEntity(ids[0])
	assert.NilError(t, err)
	archID, err := manager.GetArchIDForComponents(comps)
	assert.NilError(t, err)

	gotIDs, err := manager.GetEntitiesForArchID(archID)
	assert.NilError(t, err)
	assert.Equal(t, 10, len(gotIDs))
	assert.NilError(t, manager.FinalizeTick(ctx))

	// Discard 3 entities
	assert.NilError(t, manager.RemoveEntity(ids[0]))
	assert.NilError(t, manager.RemoveEntity(ids[4]))
	assert.NilError(t, manager.RemoveEntity(ids[7]))

	gotIDs, err = manager.GetEntitiesForArchID(archID)
	assert.NilError(t, err)
	assert.Equal(t, 7, len(gotIDs))

	// Discard these changes (this should bring the entities back)
	manager.DiscardPending()

	gotIDs, err = manager.GetEntitiesForArchID(archID)
	assert.NilError(t, err)
	assert.Equal(t, 10, len(gotIDs))
}

func TestTheRemovalOfEntitiesIsRememberedAfterReload(t *testing.T) {
	manager, client := newCmdBufferAndRedisClientForTest(t, nil)
	ctx := context.Background()

	startingIDs, err := manager.CreateManyEntities(10, fooComp, barComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.FinalizeTick(ctx))

	idToRemove := startingIDs[5]

	assert.NilError(t, manager.RemoveEntity(idToRemove))
	assert.NilError(t, manager.FinalizeTick(ctx))

	// Start a brand-new manager
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
	ctx := context.Background()

	id, err := manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	wantFoo := Foo{99}
	assert.NilError(t, manager.SetComponentForEntity(fooComp, id, wantFoo))
	gotFoo, err := manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, wantFoo, gotFoo.(Foo))

	assert.NilError(t, manager.FinalizeTick(ctx))

	assert.NilError(t, manager.RemoveComponentFromEntity(fooComp, id))

	// Make sure we can no longer get the foo component
	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.ErrorIs(t, err, storage.ErrComponentNotOnEntity)
	// But uhoh, there was a problem. This means the removal of the Foo component
	// will be undone, and the original value can be found
	manager.DiscardPending()

	gotFoo, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.Equal(t, wantFoo, gotFoo.(Foo))
}

func TestArchetypeCountTracksDiscardedChanges(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	_, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())
	assert.NilError(t, manager.FinalizeTick(ctx))

	_, err = manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	assert.Equal(t, 2, manager.ArchetypeCount())
	manager.DiscardPending()

	// The previously created archetype ID was discarded, so the count should be back to 1
	_, err = manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.Equal(t, 1, manager.ArchetypeCount())
}

func TestCannotFetchComponentOnRemovedEntityAfterCommit(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	id, err := manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
	assert.NilError(t, manager.RemoveEntity(id))

	// The entity has been removed. Trying to get a component for the entity should fail.
	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.Check(t, err != nil)

	assert.NilError(t, manager.FinalizeTick(ctx))

	// Trying to get the same component after committing to the DB should also fail.
	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.Check(t, err != nil)
}
