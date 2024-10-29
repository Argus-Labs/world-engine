package gamestate_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/gamestate/search"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	redisstorage "pkg.world.dev/world-engine/cardinal/storage/redis"
	"pkg.world.dev/world-engine/cardinal/types"
)

// newStateForTest creates a gamestate.EntityCommandBuffer using the given
// redis dbStorage. If the passed in redis
// dbStorage is nil, a redis dbStorage is world.Created.
func newStateForTest(t *testing.T, client *redis.Client) *gamestate.State {
	if client == nil {
		s := miniredis.RunT(t)
		options := redis.Options{
			Addr:     s.Addr(),
			Password: "", // no password set
			DB:       0,  // use default DB
		}

		client = redis.NewClient(&options)
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

	return state
}

func TestFinalizedState_CanGetComponent(t *testing.T) {
	state := newStateForTest(t, nil)
	ecb, fs := state.ECB(), state.FinalizedState()
	ctx := context.Background()

	id, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)

	// A read-only operation here should NOT find the entity (because it hasn't been committed yet)
	_, err = fs.GetComponentForEntity(Foo{}, id)
	assert.Check(t, err != nil)

	assert.NilError(t, ecb.FinalizeTick(ctx))

	_, err = fs.GetComponentForEntity(Foo{}, id)
	assert.NilError(t, err)
}

func TestFinalizedState_CanGetComponentTypesForEntityAndArchID(t *testing.T) {
	state := newStateForTest(t, nil)
	ecb, fs := state.ECB(), state.FinalizedState()
	ctx := context.Background()

	testCases := []struct {
		name  string
		comps []types.Component
	}{
		{
			"just foo",
			[]types.Component{Foo{}},
		},
		{
			"just bar",
			[]types.Component{Bar{}},
		},
		{
			"foo and bar",
			[]types.Component{Foo{}, Bar{}},
		},
	}

	for _, tc := range testCases {
		id, err := ecb.CreateEntity(tc.comps...)
		assert.NilError(t, err)

		gotComps, err := ecb.GetAllComponentsForEntityInRawJSON(id)
		assert.NilError(t, err)
		assert.Equal(t, len(gotComps), len(tc.comps))

		for _, comp := range tc.comps {
			compName := comp.Name()
			_, ok := gotComps[compName]
			assert.Check(t, ok, "component %q not found in entity %q", compName, id)
		}

		assert.NilError(t, ecb.FinalizeTick(ctx))

		gotComps, err = fs.GetAllComponentsForEntityInRawJSON(id)
		assert.NilError(t, err)
		assert.Equal(t, len(gotComps), len(tc.comps))

		for _, comp := range tc.comps {
			compName := comp.Name()
			_, ok := gotComps[compName]
			assert.Check(t, ok, "component %q not found in entity %q", compName, id)
		}
	}
}

func TestFinalizedState_CanFindEntityIDAfterChangingArchetypes(t *testing.T) {
	state := newStateForTest(t, nil)
	ecb, fs := state.ECB(), state.FinalizedState()
	ctx := context.Background()

	id, err := ecb.CreateEntity(Foo{})
	assert.NilError(t, err)
	assert.NilError(t, ecb.FinalizeTick(ctx))

	gotIDs, err := search.New(fs, filter.Exact(Foo{})).Collect()
	assert.NilError(t, err)
	assert.Equal(t, 1, len(gotIDs))
	assert.Equal(t, gotIDs[0], id)

	assert.NilError(t, ecb.AddComponentToEntity(Bar{}, id))
	assert.NilError(t, ecb.FinalizeTick(ctx))

	// There should be no more entities with JUST the foo componnet
	gotIDs, err = search.New(fs, filter.Exact(Foo{})).Collect()
	assert.NilError(t, err)
	assert.Equal(t, 0, len(gotIDs))

	// There should be exactly one entity with both foo and bar
	gotIDs, err = search.New(fs, filter.Exact(Foo{}, Bar{})).Collect()
	assert.NilError(t, err)
	assert.Equal(t, 1, len(gotIDs))
	assert.Equal(t, gotIDs[0], id)
}

func TestFinalizedState_ArchetypeCount(t *testing.T) {
	state := newStateForTest(t, nil)
	ecb, fs := state.ECB(), state.FinalizedState()
	ctx := context.Background()

	// No archetypes have been world.Created yet
	archCount, err := fs.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 0, archCount)

	_, err = ecb.CreateEntity(Foo{})
	assert.NilError(t, err)

	// The manager knows about the new archetype
	archCount, err = ecb.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 1, archCount)
	// but the read-only manager is not aware of it yet
	archCount, err = fs.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 0, archCount)

	assert.NilError(t, ecb.FinalizeTick(ctx))
	archCount, err = fs.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 1, archCount)

	_, err = ecb.CreateEntity(Foo{}, Bar{})
	assert.NilError(t, err)
	assert.NilError(t, ecb.FinalizeTick(ctx))
	archCount, err = fs.ArchetypeCount()
	assert.NilError(t, err)
	assert.Equal(t, 2, archCount)
}

func TestFinalizedState_FindArchetypes(t *testing.T) {
	state := newStateForTest(t, nil)
	ecb, fs := state.ECB(), state.FinalizedState()
	ctx := context.Background()

	fs.FindArchetypes(filter.Contains(filter.Component[Health]()))

	_, err := ecb.CreateManyEntities(8, Foo{})
	assert.NilError(t, err)
	_, err = ecb.CreateManyEntities(9, Bar{})
	assert.NilError(t, err)
	_, err = ecb.CreateManyEntities(10, Foo{}, Bar{})
	assert.NilError(t, err)

	componentFilter := filter.Contains(filter.Component[Bar]())

	// Before FinalizeTick is called, there should be no archetypes available to the read-only
	// manager
	archetypes, err := fs.FindArchetypes(componentFilter)
	assert.Equal(t, 0, len(archetypes))

	// Commit the archetypes to the DB
	assert.NilError(t, ecb.FinalizeTick(ctx))

	// Exactly 2 archetypes contain the Barcomponent
	archetypes, err = fs.FindArchetypes(componentFilter)
	assert.Equal(t, 2, len(archetypes))
}
