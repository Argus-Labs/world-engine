package tests

import (
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"gotest.tools/v3/assert"
)

func TestTickHappyPath(t *testing.T) {
	rs := miniredis.RunT(t)
	oneWorld := initWorldWithRedis(t, rs)
	oneEnergy := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneEnergy))

	for i := 0; i < 10; i++ {
		assert.NilError(t, oneWorld.Tick())
	}

	assert.Equal(t, 10, oneWorld.CurrentTick())

	twoWorld := initWorldWithRedis(t, rs)
	twoEnergy := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, twoWorld.RegisterComponents(twoEnergy))
	assert.Equal(t, 10, twoWorld.CurrentTick())
}

func TestErrorWhenLoadingProblematicState(t *testing.T) {
	rs := miniredis.RunT(t)
	oneWorld := initWorldWithRedis(t, rs)
	oneEnergy := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneEnergy))

	bomb := 10
	errorSystem := errors.New("some system error")
	oneWorld.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) {
		bomb--
		if bomb == 0 {
			world.LogError(errorSystem)
			return
		}
	})

	for i := 0; i < 20; i++ {
		err := oneWorld.Tick()
		if bomb > 0 {
			assert.NilError(t, err)
		} else {
			assert.ErrorContains(t, errorSystem, err.Error())
			break
		}
	}
	// Logging a world error results in a bad state. Making a new world (e.g. killing and re-running the server)
	// should notice the bad state.

	twoWorld := initWorldWithRedis(t, rs)
	twoEnergy := ecs.NewComponentType[EnergyComponent]()
	assert.ErrorIs(t, ecs.ErrorStoreStateInvalid, twoWorld.RegisterComponents(twoEnergy))
}
