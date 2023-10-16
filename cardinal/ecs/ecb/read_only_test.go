package ecb_test

import (
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

func TestReadOnly_CanGetComponent(t *testing.T) {
	manager := newCmdBufferForTest(t)

	id, err := manager.CreateEntity(fooComp)
	assert.NilError(t, err)

	_, err = manager.GetComponentForEntity(fooComp, id)
	assert.NilError(t, err)

	roStore := manager.NewReadOnlyStore()

	// A read-only operation here should NOT find the entity (because it hasn't been commited yet)
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
		comps []component.IComponentType
	}{
		{
			"just foo",
			[]component.IComponentType{fooComp},
		},
		{
			"just bar",
			[]component.IComponentType{barComp},
		},
		{
			"foo and bar",
			[]component.IComponentType{fooComp, barComp},
		},
	}

	for _, tc := range testCases {
		id, err := manager.CreateEntity(tc.comps...)
		assert.NilError(t, err)
		assert.NilError(t, manager.CommitPending())

		roStore := manager.NewReadOnlyStore()

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
		comps       []component.IComponentType
	}{
		{
			"only foo",
			3,
			[]component.IComponentType{fooComp},
		},
		{
			"only bar",
			4,
			[]component.IComponentType{barComp},
		},
		{
			"foo and bar",
			5,
			[]component.IComponentType{fooComp, barComp},
		},
	}

	roManager := manager.NewReadOnlyStore()
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

	fooArchID, err := manager.GetArchIDForComponents([]component.IComponentType{fooComp})
	assert.NilError(t, err)

	roManager := manager.NewReadOnlyStore()

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

	bothArchID, err := roManager.GetArchIDForComponents([]component.IComponentType{fooComp, barComp})
	assert.NilError(t, err)

	gotIDs, err = roManager.GetEntitiesForArchID(bothArchID)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(gotIDs))
	assert.Equal(t, gotIDs[0], id)
}

func TestReadOnly_ArchetypeCount(t *testing.T) {
	manager := newCmdBufferForTest(t)
	roManager := manager.NewReadOnlyStore()

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

func TestReadOnly_CanBeUsedInQuery(t *testing.T) {
	// TODO: The read-only version of SearchFrom is not tested because it would be best to test it
	// using a proper query and filter, but those method require a store.IManager, not a store.IReader
}
