package cardinal_test

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/test_utils"
)

func makeWorldAndTicker(t *testing.T) (world *cardinal.World, doTick func()) {
	startTickCh, doneTickCh := make(chan time.Time), make(chan uint64)
	world, err := cardinal.NewMockWorld(
		cardinal.WithTickChannel(startTickCh),
		cardinal.WithTickDoneChannel(doneTickCh))
	t.Cleanup(func() {
		world.ShutDown()
	})
	assert.NilError(t, err)

	return world, func() {
		startTickCh <- time.Now()
		<-doneTickCh
	}
}

type Foo struct{}

func (Foo) Name() string { return "foo" }

func TestCanQueryInsideSystem(t *testing.T) {
	test_utils.SetTestTimeout(t, 10*time.Second)

	world, doTick := makeWorldAndTicker(t)
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
	go func() {
		_ = world.StartGame()
	}()
	for !world.IsGameRunning() {
		time.Sleep(time.Second) //starting game async, must wait until game is running before testing everything.
	}
	doTick()

	err := world.ShutDown()
	assert.Assert(t, err)
	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}
