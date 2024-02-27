package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestDebugStateQuery(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	cardinal.MustRegisterComponent[barComp](world)
	cardinal.MustRegisterComponent[fooComp](world)

	type barFooEntity struct {
		barComp
		fooComp
	}

	entities := make([]barFooEntity, 0)
	entities = append(entities,
		barFooEntity{
			barComp{true, 320},
			fooComp{"lol", 39},
		},
		barFooEntity{
			barComp{false, 3209352835},
			fooComp{"omg", -23},
		},
	)

	err := cardinal.RegisterSystems(world, func(ctx cardinal.WorldContext) error {
		for _, entity := range entities {
			_, err := cardinal.Create(ctx, entity.barComp, entity.fooComp)
			assert.NilError(t, err)
		}
		return nil
	})
	assert.NilError(t, err)
	tf.DoTick()

	qry, err := world.GetQueryByName("state")
	assert.NilError(t, err)

	res, err := qry.HandleQuery(cardinal.NewReadOnlyWorldContext(world), cardinal.DebugStateRequest{})
	assert.NilError(t, err)

	results := *res.(*cardinal.DebugStateResponse)

	bar, err := world.GetComponentByName(barComp{}.Name())
	assert.NilError(t, err)

	foo, err := world.GetComponentByName(fooComp{}.Name())
	assert.NilError(t, err)

	assert.Len(t, results, 2)
	for i, result := range results {
		expectedBar := entities[i].barComp
		expectedFoo := entities[i].fooComp
		actualBar := result.Components[0]
		actualFoo := result.Components[1]

		assert.Equal(t, bar.ID(), actualBar.ID)
		assert.Equal(t, foo.ID(), actualFoo.ID)

		assert.Equal(t, expectedBar.Name(), actualBar.Name)
		assert.Equal(t, expectedFoo.Name(), actualFoo.Name)

		barData, err := bar.Decode(actualBar.Component)
		assert.NilError(t, err)
		fooData, err := foo.Decode(actualFoo.Component)
		assert.NilError(t, err)

		assert.Equal(t, barData.(barComp), expectedBar)
		assert.Equal(t, fooData.(fooComp), expectedFoo)
	}
}

func TestDebugStateQuery_NoState(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	qry, err := world.GetQueryByName("state")
	assert.NilError(t, err)

	res, err := qry.HandleQuery(cardinal.NewReadOnlyWorldContext(world), cardinal.DebugStateRequest{})
	assert.NilError(t, err)
	result, ok := res.(*cardinal.DebugStateResponse)
	assert.True(t, ok)

	assert.Len(t, *result, 0)
}
