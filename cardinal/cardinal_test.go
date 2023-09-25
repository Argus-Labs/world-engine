package cardinal_test

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal"
)

func TestCanQueryInsideSystem(t *testing.T) {
	type Foo struct{}
	nextTickCh := make(chan time.Time)
	tickDoneCh := make(chan uint64)

	world, err := cardinal.NewMockWorld(
		cardinal.WithTickChannel(nextTickCh),
		cardinal.WithTickDoneChannel(tickDoneCh))
	assert.NilError(t, err)
	comp := cardinal.NewComponentType[Foo]("foo")
	world.RegisterComponents(comp)

	wantNumOfEntities := 10
	_, err = world.CreateMany(wantNumOfEntities, comp)
	assert.NilError(t, err)
	gotNumOfEntities := 0
	world.RegisterSystems(func(world *cardinal.World, queue *cardinal.TransactionQueue, logger *cardinal.Logger) error {
		cardinal.NewQuery(cardinal.Exact(comp)).Each(world, func(cardinal.EntityID) bool {
			gotNumOfEntities++
			return true
		})
		return nil
	})
	go func() {
		world.StartGame()
	}()
	nextTickCh <- time.Now()
	<-tickDoneCh

	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}
