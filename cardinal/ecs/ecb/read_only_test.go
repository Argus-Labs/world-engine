package ecb_test

import (
	"context"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"testing"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/component"
)

func TestReadOnly_CanGetComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)

	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)

	roStore := manager.ToReadOnly()

	// A read-only operation here should NOT find the entity (because it hasn't been committed yet)
	_, err = roStore.GetComponentForEntity(fooComp, id)
	assert.Check(t, err != nil)

	assert.NilError(t, manager.FinalizeTick(ctx))

	_, err = roStore.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)
}

func TestReadOnly_CanGetComponentTypesForEntityAndArchID(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	testCases := []struct {
		name  string
		comps []component.ComponentMetadata
	}{
		{
			"just foo",
			[]component.ComponentMetadata{fooComp},
		},
		{
			"just bar",
			[]component.ComponentMetadata{barComp},
		},
		{
			"foo and bar",
			[]component.ComponentMetadata{fooComp, barComp},
		},
	}

	for _, tc := range testCases {
		id, err := manager.CreateEntity(tc.comps...)
		assert.NilError(t, err)
		assert.NilError(t, manager.FinalizeTick(ctx))

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
	ctx := context.Background()
	testCases := []struct {
		name        string
		idsToCreate int
		comps       []component.ComponentMetadata
	}{
		{
			"only foo",
			3,
			[]component.ComponentMetadata{fooComp},
		},
		{
			"only bar",
			4,
			[]component.ComponentMetadata{barComp},
		},
		{
			"foo and bar",
			5,
			[]component.ComponentMetadata{fooComp, barComp},
		},
	}

	roManager := manager.ToReadOnly()
	for _, tc := range testCases {
		ids, err := manager.CreateManyEntities(tc.idsToCreate, tc.comps...)
		assert.NilError(t, err)
		assert.NilError(t, manager.FinalizeTick(ctx))

		archID, err := roManager.GetArchIDForComponents(tc.comps)
		assert.NilError(t, err)

		gotIDs, err := roManager.GetEntitiesForArchID(archID)
		assert.NilError(t, err)
		assert.DeepEqual(t, ids, gotIDs)
	}
}

func TestReadOnly_CanFindEntityIDAfterChangingArchetypes(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()
	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.FinalizeTick(ctx))

	fooArchID, err := manager.GetArchIDForComponents([]component.ComponentMetadata{fooComp})
	assert.NilError(t, err)

	roManager := manager.ToReadOnly()

	gotIDs, err := roManager.GetEntitiesForArchID(fooArchID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(gotIDs))
	assert.Equal(t, gotIDs[0], id)

	assert.NilError(t, manager.AddComponentToEntity(barComp, id))
	assert.NilError(t, manager.FinalizeTick(ctx))

	// There should be no more entities with JUST the foo componnet
	gotIDs, err = roManager.GetEntitiesForArchID(fooArchID)
	assert.NilError(t, err)
	assert.Equal(t, 0, len(gotIDs))

	bothArchID, err := roManager.GetArchIDForComponents([]component.ComponentMetadata{fooComp, barComp})
	assert.NilError(t, err)

	gotIDs, err = roManager.GetEntitiesForArchID(bothArchID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(gotIDs))
	assert.Equal(t, gotIDs[0], id)
}

func TestReadOnly_ArchetypeCount(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()
	roManager := manager.ToReadOnly()

	// No archetypes have been created yet
	assert.Equal(t, 0, roManager.ArchetypeCount())

	_, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)
	// The manager knows about the new archetype
	assert.Equal(t, 1, manager.ArchetypeCount())
	// but the read-only manager is not aware of it yet
	assert.Equal(t, 0, roManager.ArchetypeCount())

	assert.NilError(t, manager.FinalizeTick(ctx))
	assert.Equal(t, 1, roManager.ArchetypeCount())

	_, err = manager.CreateEntity(fooComp, barComp)
	assert.NilError(t, err)
	assert.NilError(t, manager.FinalizeTick(ctx))
	assert.Equal(t, 2, roManager.ArchetypeCount())
}

func TestReadOnly_SearchFrom(t *testing.T) {
	manager := newCmdBufferForTest(t)
	ctx := context.Background()

	engine := testutils.NewTestWorld(t, cardinal.WithStoreManager(manager)).Engine()
	assert.NilError(t, ecs.RegisterComponent[Health](engine))
	assert.NilError(t, ecs.RegisterComponent[Power](engine))
	assert.NilError(t, engine.LoadGameState())

	eCtx := ecs.NewEngineContext(engine)
	_, err := ecs.CreateMany(eCtx, 8, Health{})
	assert.NilError(t, err)
	_, err = ecs.CreateMany(eCtx, 9, Power{})
	assert.NilError(t, err)
	_, err = ecs.CreateMany(eCtx, 10, Health{}, Power{})
	assert.NilError(t, err)

	componentFilter := filter.Contains(Health{})

	roManager := manager.ToReadOnly()

	// Before FinalizeTick is called, there should be no archetypes available to the read-only
	// manager
	archetypeIter := roManager.SearchFrom(componentFilter, 0)
	assert.Equal(t, 0, len(archetypeIter.Values))

	// Commit the archetypes to the DB
	assert.NilError(t, manager.FinalizeTick(ctx))

	// Exactly 2 archetypes contain the Health component
	archetypeIter = roManager.SearchFrom(componentFilter, 0)
	assert.Equal(t, 2, len(archetypeIter.Values))
}
