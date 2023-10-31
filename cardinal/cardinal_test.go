package cardinal_test

import (
	"pkg.world.dev/world-engine/cardinal/testutils"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal"
)

type Foo struct{}

func (Foo) Name() string { return "foo" }

func TestCanQueryInsideSystem(t *testing.T) {
	testutils.SetTestTimeout(t, 10*time.Second)

	world, doTick := testutils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	wantNumOfEntities := 10
	world.Init(func(worldCtx cardinal.WorldContext) {
		_, err := cardinal.CreateMany(worldCtx, wantNumOfEntities, Foo{})
		assert.NilError(t, err)
	})
	gotNumOfEntities := 0
	cardinal.RegisterSystems(world, func(worldCtx cardinal.WorldContext) error {
		q, err := worldCtx.NewSearch(cardinal.Exact(Foo{}))
		assert.NilError(t, err)
		err = q.Each(worldCtx, func(cardinal.EntityID) bool {
			gotNumOfEntities++
			return true
		})
		assert.NilError(t, err)
		return nil
	})

	doTick()

	err := world.ShutDown()
	assert.Assert(t, err)
	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}
