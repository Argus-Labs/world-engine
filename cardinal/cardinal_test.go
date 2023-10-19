package cardinal_test

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

// TODO this function needs to be moved to a utils package above cardinal to prevent circulars.
func setTestTimeout(t *testing.T, timeout time.Duration) {
	if _, ok := t.Deadline(); ok {
		// A deadline has already been set. Don't add an additional deadline.
		return
	}
	success := make(chan bool)
	t.Cleanup(func() {
		success <- true
	})
	go func() {
		select {
		case <-success:
			// test was successful. Do nothing
		case <-time.After(timeout):
			//assert.Check(t, false, "test timed out")
			panic("test timed out")
		}
	}()
}

type Foo struct{}

func (Foo) Name() string { return "foo" }

func TestCanQueryInsideSystem(t *testing.T) {
	setTestTimeout(t, 10*time.Second)

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
		q, err := wCtx.NewSearch(ecs.Exact(Foo{}))
		assert.NilError(t, err)
		q.Each(wCtx, func(cardinal.EntityID) bool {
			gotNumOfEntities++
			return true
		})
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
