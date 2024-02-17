package cardinal_test

import (
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"testing"
)

type fooComp struct {
	X string
	Y int
}

func (fooComp) Name() string { return "foo" }

type barComp struct {
	Z bool
	R uint64
}

func (barComp) Name() string { return "bar" }

func TestCQLQuery(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	cardinal.MustRegisterComponent[barComp](world)
	cardinal.MustRegisterComponent[fooComp](world)

	firstBar := barComp{true, 420}
	secondBar := barComp{false, 20}
	bars := []barComp{firstBar, secondBar}
	err := cardinal.RegisterSystems(world, func(ctx cardinal.WorldContext) error {
		_, err := cardinal.Create(ctx, firstBar, fooComp{"hi", 32})
		assert.NilError(t, err)
		_, err = cardinal.Create(ctx, secondBar)
		assert.NilError(t, err)
		_, err = cardinal.Create(ctx, fooComp{"no", 33})
		assert.NilError(t, err)
		return nil
	})
	assert.NilError(t, err)
	tf.DoTick()

	barComponent, err := world.GetComponentByName(barComp{}.Name())
	assert.NilError(t, err)

	query, err := world.GetQueryByName("cql")
	assert.NilError(t, err)

	res, err := query.HandleQuery(cardinal.NewReadOnlyWorldContext(world), cardinal.CQLQueryRequest{CQL: "CONTAINS(bar)"})
	assert.NilError(t, err)
	result, ok := res.(*cardinal.CQLQueryResponse)
	assert.True(t, ok)

	assert.Len(t, result.Results, 2)

	for i, r := range result.Results {
		gotBarAny, err := barComponent.Decode(r.Data[0])
		assert.NilError(t, err)
		gotBar, ok := gotBarAny.(barComp)
		assert.True(t, ok)
		assert.Equal(t, gotBar, bars[i])
	}
}

func TestCQLQueryErrorOnBadFormat(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	query, err := world.GetQueryByName("cql")
	assert.NilError(t, err)
	res, err := query.HandleQuery(cardinal.NewReadOnlyWorldContext(world), cardinal.CQLQueryRequest{CQL: "MEOW(FOO)"})
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "failed to parse CQL string")
}

func TestCQLQueryNonExistentComponent(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	query, err := world.GetQueryByName("cql")
	assert.NilError(t, err)
	res, err := query.HandleQuery(cardinal.NewReadOnlyWorldContext(world), cardinal.CQLQueryRequest{CQL: "CONTAINS(meow)"})
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), `component "meow" must be registered before being used`)
}
