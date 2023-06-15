package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"github.com/redis/go-redis/v9"
	"gotest.tools/v3/assert"
)

func TestTickHappyPath(t *testing.T) {
	rs := miniredis.RunT(t)
	oneWorld := initWorldWithRedis(t, rs)
	oneEnergy := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(oneEnergy))
	assert.NilError(t, oneWorld.LoadGameState())

	for i := 0; i < 10; i++ {
		assert.NilError(t, oneWorld.Tick())
	}

	assert.Equal(t, 10, oneWorld.CurrentTick())

	twoWorld := initWorldWithRedis(t, rs)
	twoEnergy := ecs.NewComponentType[EnergyComponent]()
	assert.NilError(t, twoWorld.RegisterComponents(twoEnergy))
	assert.NilError(t, twoWorld.LoadGameState())
	assert.Equal(t, 10, twoWorld.CurrentTick())
}

func TestCanIdentifyAndFixSystemError(t *testing.T) {
	type PowerComponent struct {
		Power int
	}

	rs := miniredis.RunT(t)
	oneWorld := initWorldWithRedis(t, rs)
	onePower := ecs.NewComponentType[PowerComponent]()
	assert.NilError(t, oneWorld.RegisterComponents(onePower))

	id, err := oneWorld.Create(onePower)
	assert.NilError(t, err)

	errorSystem := errors.New("3 power? That's too much, man!")

	// In this test, our "buggy" system fails once Power reaches 3
	oneWorld.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		p, err := onePower.Get(world, id)
		if err != nil {
			return err
		}
		p.Power++
		if p.Power >= 3 {
			return errorSystem
		}
		return onePower.Set(world, id, &p)
	})
	assert.NilError(t, oneWorld.LoadGameState())

	// Power is set to 1
	assert.NilError(t, oneWorld.Tick())
	// Power is set to 2
	assert.NilError(t, oneWorld.Tick())
	// Power is set to 3, then the System fails
	assert.ErrorIs(t, errorSystem, oneWorld.Tick())

	// Set up a new world using the same storage layer
	twoWorld := initWorldWithRedis(t, rs)
	twoPower := ecs.NewComponentType[*PowerComponent]()
	assert.NilError(t, twoWorld.RegisterComponents(twoPower))

	// this is our fixed system that can handle Power levels of 3 and higher
	twoWorld.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		p, err := onePower.Get(world, id)
		if err != nil {
			return err
		}
		p.Power++
		return onePower.Set(world, id, &p)
	})

	// Loading a game state with the fixed system should automatically finish the previous tick.
	assert.NilError(t, twoWorld.LoadGameState())
	p, err := onePower.Get(twoWorld, id)
	assert.NilError(t, err)
	assert.Equal(t, 3, p.Power)

	// Just for func, tick one last time to make sure power is still being incremented.
	assert.NilError(t, twoWorld.Tick())
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

	alpha.Set(world, wantID, &wantScalar)

	verifyCanFindEntity := func() {
		// Make sure we can find the entityj
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

func TestMiniRedis(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:     miniredis.RunT(t).Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	ctx := context.Background()

	assert.NilError(t, client.LPush(ctx, "A", "original").Err())
	assert.NilError(t, client.Copy(ctx, "A", "B", 0, true).Err())
	assert.NilError(t, client.LSet(ctx, "A", 0, "modify").Err())

	aVal, err := client.LIndex(ctx, "A", 0).Result()
	assert.NilError(t, err)
	assert.Equal(t, aVal, "modify")

	bVal, err := client.LIndex(ctx, "B", 0).Result()
	assert.NilError(t, err)
	assert.Equal(t, bVal, "original")
}

func TestCanRecoverStateAfterFailedArchetypeChange(t *testing.T) {
	type ScalarComponent struct {
		Val int
	}
	rs := miniredis.RunT(t)
	for _, firstWorldIteration := range []bool{true, false} {
		world := initWorldWithRedis(t, rs)
		static := ecs.NewComponentType[ScalarComponent]()
		toggle := ecs.NewComponentType[ScalarComponent]()
		assert.NilError(t, world.RegisterComponents(static, toggle))

		if firstWorldIteration {
			_, err := world.Create(static)
			assert.NilError(t, err)
		}

		errorToggleComponent := errors.New("problem with toggle component")
		world.AddSystem(func(w *ecs.World, _ *ecs.TransactionQueue) error {
			// Get the one and only entity ID
			id, ok, err := static.First(w)
			assert.NilError(t, err)
			assert.Check(t, ok)

			s, err := static.Get(w, id)
			assert.NilError(t, err)
			s.Val++
			assert.NilError(t, static.Set(w, id, &s))
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
				assert.NilError(t, world.Tick())
			}
			// After 4 ticks, static.Val should be 4 and toggle should have just been removed from the entity.
			_, err := toggle.Get(world, id)
			assert.ErrorIs(t, storage.ErrorComponentNotOnEntity, err)

			// Ticking again should result in an error
			assert.ErrorIs(t, errorToggleComponent, world.Tick())
		} else {
			// At this second iteration, the errorToggleComponent bug has been fixed. static.Val should be 5
			// and it should have just been added to the entity.
			_, err := toggle.Get(world, id)
			assert.NilError(t, err)
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
		world := initWorldWithRedis(t, rs)

		powerComp := ecs.NewComponentType[FloatValue]()
		assert.NilError(t, world.RegisterComponents(powerComp))

		powerTx := ecs.NewTransactionType[FloatValue]()
		assert.NilError(t, world.RegisterTransactions(powerTx))

		world.AddSystem(func(w *ecs.World, queue *ecs.TransactionQueue) error {
			id, err := powerComp.MustFirst(w)
			assert.NilError(t, err)
			entityPower, err := powerComp.Get(w, id)

			changes := powerTx.In(queue)
			assert.Equal(t, 1, len(changes))
			entityPower.Val += changes[0].Val
			assert.NilError(t, powerComp.Set(w, id, &entityPower))

			if isBuggyIteration && changes[0].Val == 666 {
				return errorBadPowerChange
			}
			return nil
		})
		assert.NilError(t, world.LoadGameState())

		_, err := world.Create(powerComp)
		assert.NilError(t, err)

		if isBuggyIteration {
			// perform a few ticks that will not result in an error
			powerTx.AddToQueue(world, &FloatValue{1000})
			assert.NilError(t, world.Tick())
			powerTx.AddToQueue(world, &FloatValue{1000})
			assert.NilError(t, world.Tick())
			powerTx.AddToQueue(world, &FloatValue{1000})
			assert.NilError(t, world.Tick())

			powerTx.AddToQueue(world, &FloatValue{666})
			assert.ErrorIs(t, errorBadPowerChange, world.Tick())
		} else {
			// Loading the game state above should

		}

	}
}

func doTwoTimes(fn func(firstIteration bool)) {
	for _, first := range []bool{true, false} {
		fn(first)
	}
}
