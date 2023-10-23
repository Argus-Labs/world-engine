package cardinal_test

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/test_utils"
)

type Foo struct{}

func (Foo) Name() string { return "foo" }

func TestCanQueryInsideSystem(t *testing.T) {
	test_utils.SetTestTimeout(t, 10*time.Second)

	nextTickCh := make(chan time.Time)
	tickDoneCh := make(chan uint64)

	world, err := cardinal.NewMockWorld(
		cardinal.WithTickChannel(nextTickCh),
		cardinal.WithTickDoneChannel(tickDoneCh))
	assert.NilError(t, err)
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	wantNumOfEntities := 10
	world.Init(func(wCtx cardinal.WorldContext) {
		_, err = cardinal.CreateMany(wCtx, wantNumOfEntities, Foo{})
		assert.NilError(t, err)
	})
	gotNumOfEntities := 0
	cardinal.RegisterSystems(world, func(wCtx cardinal.WorldContext) error {
		q, err := wCtx.NewSearch(cardinal.Exact(Foo{}))
		assert.NilError(t, err)
		err = q.Each(wCtx, func(cardinal.EntityID) bool {
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
	nextTickCh <- time.Now()
	<-tickDoneCh
	err = world.ShutDown()
	assert.Assert(t, err)
	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}
