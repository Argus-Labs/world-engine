package ecb_test

import (
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
)

func TestReadOnly_CanGetComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)

	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)

	roStore := manager.ToReadOnly()

	// A read-only operation here should NOT find the entity (because it hasn't been committed yet)
	_, err = roStore.GetComponentForEntity(fooComp, id)
	assert.Check(t, err != nil)

	assert.NilError(t, manager.CommitPending())

	_, err = roStore.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
}

func TestReadOnly_CanGetComponentTypesForEntityAndArchID(t *testing.T) {
	manager := newCmdBufferForTest(t)

	testCases := []struct {
		name  string
		comps []metadata.ComponentMetadata
	}{
		{
			"just foo",
			[]metadata.ComponentMetadata{fooComp},
		},
		{
			"just bar",
			[]metadata.ComponentMetadata{barComp},
		},
		{
			"foo and bar",
			[]metadata.ComponentMetadata{fooComp, barComp},
		},
	}

	for _, tc := range testCases {
		id, err := manager.CreateEntity(tc.comps...)
		assert.NilError(t, err)
		assert.NilError(t, manager.CommitPending())

		roStore := manager.ToReadOnly()

		gotComps, err := roStore.GetComponentTypesForEntity(id)
		assert.NilError(t, err)
		assert.Equal(t, len(gotComps), len(tc.comps))
		for i := range gotComps {
			assert.Equal(t, gotComps[i].ID(), tc.comps[i].ID(), "component mismatch for test case %q", tc.name)
		}

		archID, err := roStore.GetArchIDForComponents(gotComps)
		assert.NilError(t, err)
		gotComps = roStore.GetComponentTypesForArchID(archID)
		assert.NilError(t, err)
		assert.Equal(t, len(gotComps), len(tc.comps))
		for i := range gotComps {
			assert.Equal(t, gotComps[i].ID(), tc.comps[i].ID(), "component mismatch for test case %q", tc.name)
		}
	}
}

func TestReadOnly_GetEntitiesForArchID(t *testing.T) {
	manager := newCmdBufferForTest(t)
	testCases := []struct {
		name        string
		idsToCreate int
		comps       []metadata.ComponentMetadata
	}{
		{
			"only foo",
			3,
			[]metadata.ComponentMetadata{fooComp},
		},
		{
			"only bar",
			4,
			[]metadata.ComponentMetadata{barComp},
		},
		{
			"foo and bar",
			5,
			[]metadata.ComponentMetadata{fooComp, barComp},
		},
	}

	roManager := manager.ToReadOnly()
	for _, tc := range testCases {
		ids, err := manager.CreateManyEntities(tc.idsToCreate, tc.comps...)
		assert.NilError(t, err)
		assert.NilError(t, manager.CommitPending())

		archID, err := roManager.GetArchIDForComponents(tc.comps)
		assert.NilError(t, err)

		gotIDs, err := roManager.GetEntitiesForArchID(archID)
		assert.NilError(t, err)
		assert.DeepEqual(t, ids, gotIDs)
	}
}

func TestReadOnly_CanFindEntityIDAfterChangingArchetypes(t *testing.T) {
	manager := newCmdBufferForTest(t)
	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.CommitPending())

	fooArchID, err := manager.GetArchIDForComponents([]metadata.ComponentMetadata{fooComp})
	assert.NilError(t, err)

	roManager := manager.ToReadOnly()

	gotIDs, err := roManager.GetEntitiesForArchID(fooArchID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(gotIDs))
	assert.Equal(t, gotIDs[0], id)

	assert.NilError(t, manager.AddComponentToEntity(barComp, id))
	assert.NilError(t, manager.CommitPending())

	// There should be no more entities with JUST the foo componnet
	gotIDs, err = roManager.GetEntitiesForArchID(fooArchID)
	assert.NilError(t, err)
	assert.Equal(t, 0, len(gotIDs))

	bothArchID, err := roManager.GetArchIDForComponents([]metadata.ComponentMetadata{fooComp, barComp})
	assert.NilError(t, err)

	gotIDs, err = roManager.GetEntitiesForArchID(bothArchID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(gotIDs))
	assert.Equal(t, gotIDs[0], id)
}

func TestReadOnly_ArchetypeCount(t *testing.T) {
	manager := newCmdBufferForTest(t)
	roManager := manager.ToReadOnly()

	// No archetypes have been created yet
	assert.Equal(t, 0, roManager.ArchetypeCount())

	_, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	// The manager knows about the new archetype
	assert.Equal(t, 1, manager.ArchetypeCount())
	// but the read-only manager is not aware of it yet
	assert.Equal(t, 0, roManager.ArchetypeCount())

	assert.NilError(t, manager.CommitPending())
	assert.Equal(t, 1, roManager.ArchetypeCount())

	_, err = manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.CommitPending())
	assert.Equal(t, 2, roManager.ArchetypeCount())
}

func TestReadOnly_SearchFrom(t *testing.T) {
	manager := newCmdBufferForTest(t)

	world := ecs.NewTestWorld(t, ecs.WithStoreManager(manager))
	assert.NilError(t, ecs.RegisterComponent[Health](world))
	assert.NilError(t, ecs.RegisterComponent[Power](world))
	assert.NilError(t, world.LoadGameState())

	wCtx := ecs.NewWorldContext(world)
	_, err := component.CreateMany(wCtx, 8, Health{})
	assert.NilError(t, err)
	_, err = component.CreateMany(wCtx, 9, Power{})
	assert.NilError(t, err)
	_, err = component.CreateMany(wCtx, 10, Health{}, Power{})
	assert.NilError(t, err)

	filter := ecs.Contains(Health{})
	componentFilter, err := filter.ConvertToComponentFilter(world)
	assert.NilError(t, err)

	roManager := manager.ToReadOnly()

	// Before CommitPending is called, there should be no archetypes available to the read-only
	// manager
	archetypeIter := roManager.SearchFrom(componentFilter, 0)
	assert.Equal(t, 0, len(archetypeIter.Values))

	// Commit the archetypes to the DB
	assert.NilError(t, manager.CommitPending())

	// Exactly 2 archetypes contain the Health component
	archetypeIter = roManager.SearchFrom(componentFilter, 0)
	assert.Equal(t, 2, len(archetypeIter.Values))
}
