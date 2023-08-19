package ecs_test

import (
	"context"
	"errors"
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

func TestTickHappyPath(t *testing.T) {
	rs := miniredis.RunT(t)
	oneWorld := testutil.InitWorldWithRedis(t, rs)
	oneEnergy := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneEnergy))
	assert.NilError(t, oneWorld.LoadGameState())

	for i := 0; i < 10; i++ {
		assert.NilError(t, oneWorld.Tick(context.Background()))
	}

	assert.Equal(t, uint64(10), oneWorld.CurrentTick())

	twoWorld := testutil.InitWorldWithRedis(t, rs)
	twoEnergy := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, twoWorld.RegisterComponents(twoEnergy))
	assert.NilError(t, twoWorld.LoadGameState())
	assert.Equal(t, uint64(10), twoWorld.CurrentTick())
}

func TestCanIdentifyAndFixSystemError(t *testing.T) {
	type PowerComponent struct {
		Power int
	}

	rs := miniredis.RunT(t)
	oneWorld := testutil.InitWorldWithRedis(t, rs)
	onePower := ecs.NewComponentType[PowerComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(onePower))

	id, err := oneWorld.Create(onePower)
	assert.NilError(t, err)

	errorSystem := errors.New("3 power? That's too much, man!")

	// In this test, our "buggy" system fails once Power reaches 3
	oneWorld.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, _ *zerolog.Logger) error {
		p, err := onePower.Get(world, id)
		if err != nil {
			return err
		}
		p.Power++
		if p.Power >= 3 {
			return errorSystem
		}
		return onePower.Set(world, id, p)
	})
	assert.NilError(t, oneWorld.LoadGameState())

	// Power is set to 1
	assert.NilError(t, oneWorld.Tick(context.Background()))
	// Power is set to 2
	assert.NilError(t, oneWorld.Tick(context.Background()))
	// Power is set to 3, then the System fails
	assert.ErrorIs(t, errorSystem, oneWorld.Tick(context.Background()))

	// Set up a new world using the same storage layer
	twoWorld := testutil.InitWorldWithRedis(t, rs)
	twoPower := ecs.NewComponentType[*PowerComponent]()
	assert.NilError(t, twoWorld.RegisterComponents(twoPower))

	// this is our fixed system that can handle Power levels of 3 and higher
	twoWorld.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, _ *zerolog.Logger) error {
		p, err := onePower.Get(world, id)
		if err != nil {
			return err
		}
		p.Power++
		return onePower.Set(world, id, p)
	})

	// Loading a game state with the fixed system should automatically finish the previous tick.
	assert.NilError(t, twoWorld.LoadGameState())
	p, err := onePower.Get(twoWorld, id)
	assert.NilError(t, err)
	assert.Equal(t, 3, p.Power)

	// Just for fun, tick one last time to make sure power is still being incremented.
	assert.NilError(t, twoWorld.Tick(context.Background()))
	p, err = onePower.Get(twoWorld, id)
	assert.NilError(t, err)
	assert.Equal(t, 4, p.Power)
}

func TestCanModifyArchetypeAndGetEntity(t *testing.T) {
	type ScalarComponent struct {
		Val int
	}
	world := inmem.NewECSWorldForTest(t)
	alpha := ecs.NewComponentType[ScalarComponent]()
	beta := ecs.NewComponentType[ScalarComponent]()
	assert.NilError(t, world.RegisterComponents(alpha))
	assert.NilError(t, world.LoadGameState())

	wantID, err := world.Create(alpha)
	assert.NilError(t, err)

	wantScalar := ScalarComponent{99}

	assert.NilError(t, alpha.Set(world, wantID, wantScalar))

	verifyCanFindEntity := func() {
		// Make sure we can find the entity
		gotID, ok, err := alpha.First(world)
		assert.NilError(t, err)
		assert.Check(t, ok)
		assert.Equal(t, wantID, gotID)

		// Make sure the associated component is correct
		gotScalar, err := alpha.Get(world, wantID)
		assert.NilError(t, err)
		assert.Equal(t, wantScalar, gotScalar)
	}

	// Make sure we can find the one-and-only entity ID
	verifyCanFindEntity()

	// Add on the beta component
	assert.NilError(t, beta.AddTo(world, wantID))
	verifyCanFindEntity()

	// Remove the beta component
	assert.NilError(t, beta.RemoveFrom(world, wantID))
	verifyCanFindEntity()
}

func TestCanRecoverStateAfterFailedArchetypeChange(t *testing.T) {
	type ScalarComponent struct {
		Val int
	}
	rs := miniredis.RunT(t)
	for _, firstWorldIteration := range []bool{true, false} {
		world := testutil.InitWorldWithRedis(t, rs)
		static := ecs.NewComponentType[ScalarComponent]()
		toggle := ecs.NewComponentType[ScalarComponent]()
		assert.NilError(t, world.RegisterComponents(static, toggle))

		if firstWorldIteration {
			_, err := world.Create(static)
			assert.NilError(t, err)
		}

		errorToggleComponent := errors.New("problem with toggle component")
		world.AddSystem(func(w *ecs.World, _ *ecs.TransactionQueue, _ *zerolog.Logger) error {
			// Get the one and only entity ID
			id, ok, err := static.First(w)
			assert.NilError(t, err)
			assert.Check(t, ok)

			s, err := static.Get(w, id)
			assert.NilError(t, err)
			s.Val++
			assert.NilError(t, static.Set(w, id, s))
			if s.Val%2 == 1 {
				assert.NilError(t, toggle.AddTo(w, id))
			} else {
				assert.NilError(t, toggle.RemoveFrom(w, id))
			}

			if firstWorldIteration && s.Val == 5 {
				return errorToggleComponent
			}

			return nil
		})
		assert.NilError(t, world.LoadGameState())

		id, ok, err := static.First(world)
		assert.NilError(t, err)
		assert.Check(t, ok)

		if firstWorldIteration {
			for i := 0; i < 4; i++ {
				assert.NilError(t, world.Tick(context.Background()))
			}
			// After 4 ticks, static.Val should be 4 and toggle should have just been removed from the entity.
			_, err := toggle.Get(world, id)
			assert.ErrorIs(t, storage.ErrorComponentNotOnEntity, err)

			// Ticking again should result in an error
			assert.ErrorIs(t, errorToggleComponent, world.Tick(context.Background()))
		} else {
			// At this second iteration, the errorToggleComponent bug has been fixed. static.Val should be 5
			// and toggle should have just been added to the entity.
			_, err := toggle.Get(world, id)
			assert.NilError(t, err)

			s, err := static.Get(world, id)
			assert.NilError(t, err)
			assert.Equal(t, 5, s.Val)
		}
	}
}

func TestCanRecoverTransactionsFromFailedSystemRun(t *testing.T) {
	type FloatValue struct {
		Val float64
	}
	rs := miniredis.RunT(t)
	errorBadPowerChange := errors.New("bad power change transaction")
	for _, isBuggyIteration := range []bool{true, false} {
		world := testutil.InitWorldWithRedis(t, rs)

		powerComp := ecs.NewComponentType[FloatValue]()
		assert.NilError(t, world.RegisterComponents(powerComp))

		powerTx := ecs.NewTransactionType[FloatValue, FloatValue]("change_power")
		assert.NilError(t, world.RegisterTransactions(powerTx))

		world.AddSystem(func(w *ecs.World, queue *ecs.TransactionQueue, _ *zerolog.Logger) error {
			id, err := powerComp.MustFirst(w)
			assert.NilError(t, err)
			entityPower, err := powerComp.Get(w, id)

			changes := powerTx.In(queue)
			assert.Equal(t, 1, len(changes))
			entityPower.Val += changes[0].Value.Val
			assert.NilError(t, powerComp.Set(w, id, entityPower))

			if isBuggyIteration && changes[0].Value.Val == 666 {
				return errorBadPowerChange
			}
			return nil
		})
		assert.NilError(t, world.LoadGameState())

		// Only create the entity for the first iteration
		if isBuggyIteration {
			_, err := world.Create(powerComp)
			assert.NilError(t, err)
		}

		// fetchPower is a small helper to get the power of the only entity in the world
		fetchPower := func() float64 {
			id, ok, err := powerComp.First(world)
			assert.Check(t, ok)
			assert.NilError(t, err)
			power, err := powerComp.Get(world, id)
			assert.NilError(t, err)
			return power.Val
		}

		if isBuggyIteration {
			// perform a few ticks that will not result in an error
			powerTx.AddToQueue(world, FloatValue{1000})
			assert.NilError(t, world.Tick(context.Background()))
			powerTx.AddToQueue(world, FloatValue{1000})
			assert.NilError(t, world.Tick(context.Background()))
			powerTx.AddToQueue(world, FloatValue{1000})
			assert.NilError(t, world.Tick(context.Background()))

			assert.Equal(t, float64(3000), fetchPower())

			// In this "buggy" iteration, the above system cannot handle a power of 666.
			powerTx.AddToQueue(world, FloatValue{666})
			assert.ErrorIs(t, errorBadPowerChange, world.Tick(context.Background()))
		} else {
			// Loading the game state above should successfully re-process that final 666 transactions.
			assert.Equal(t, float64(3666), fetchPower())

			// One more tick for good measure
			powerTx.AddToQueue(world, FloatValue{1000})
			assert.NilError(t, world.Tick(context.Background()))

			assert.Equal(t, float64(4666), fetchPower())
		}
	}
}
