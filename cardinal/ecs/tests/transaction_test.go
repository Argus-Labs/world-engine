package tests

import (
	"testing"
	"time"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"gotest.tools/v3/assert"
)

type ScoreComponent struct {
	Score int
}

type ModifyScoreTx struct {
	PlayerID storage.EntityID
	Amount   int
}

func TestCanQueueTransactions(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)

	// Create an entity with a score component
	score := ecs.NewComponentType[*ScoreComponent]()
	assert.NilError(t, world.RegisterComponents(score))
	modifyScoreTx := ecs.NewTransactionType[*ModifyScoreTx]()
	assert.NilError(t, world.RegisterTransactions(modifyScoreTx))

	id, err := world.Create(score)
	assert.NilError(t, err)

	// Set up a system that allows for the modification of a player's score
	world.AddSystem(func(w *ecs.World, queue *ecs.TransactionQueue) error {
		modifyScore := modifyScoreTx.In(queue)
		for _, ms := range modifyScore {
			err := score.Update(w, ms.PlayerID, func(s *ScoreComponent) *ScoreComponent {
				s.Score += ms.Amount
				return s
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	modifyScoreTx.AddToQueue(world, &ModifyScoreTx{id, 100})

	assert.NilError(t, score.Set(world, id, &ScoreComponent{}))

	// Verify the score is 0
	s, err := score.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 0, s.Score)

	// Process a game tick
	assert.NilError(t, world.Tick())

	// Verify the score was updated
	s, err = score.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, s.Score)

	// Tick again, but no new modifyScoreTx was added to the queue
	assert.NilError(t, world.Tick())

	// Verify the score hasn't changed
	s, err = score.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, s.Score)
}

func TestSystemsAreExecutedDuringGameTick(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type CounterComponent struct {
		Count int
	}
	count := ecs.NewComponentType[CounterComponent]()
	assert.NilError(t, world.RegisterComponents(count))

	id, err := world.Create(count)
	assert.NilError(t, err)
	world.AddSystem(func(w *ecs.World, _ *ecs.TransactionQueue) error {
		return count.Update(w, id, func(c CounterComponent) CounterComponent {
			c.Count++
			return c
		})
	})
	assert.NilError(t, world.LoadGameState())

	for i := 0; i < 10; i++ {
		assert.NilError(t, world.Tick())
	}

	c, err := count.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 10, c.Count)
}

func TestTransactionAreAppliedToSomeEntities(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	alphaScore := ecs.NewComponentType[ScoreComponent]()
	assert.NilError(t, world.RegisterComponents(alphaScore))

	modifyScoreTx := ecs.NewTransactionType[*ModifyScoreTx]()
	assert.NilError(t, world.RegisterTransactions(modifyScoreTx))

	world.AddSystem(func(w *ecs.World, queue *ecs.TransactionQueue) error {
		modifyScores := modifyScoreTx.In(queue)
		for _, ms := range modifyScores {
			err := alphaScore.Update(w, ms.PlayerID, func(s ScoreComponent) ScoreComponent {
				s.Score += ms.Amount
				return s
			})
			assert.Check(t, err == nil)
		}
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	ids, err := world.CreateMany(100, alphaScore)
	assert.NilError(t, err)
	// Entities at index 5, 10 and 50 will be updated with some values
	modifyScoreTx.AddToQueue(world, &ModifyScoreTx{
		PlayerID: ids[5],
		Amount:   105,
	})
	modifyScoreTx.AddToQueue(world, &ModifyScoreTx{
		PlayerID: ids[10],
		Amount:   110,
	})
	modifyScoreTx.AddToQueue(world, &ModifyScoreTx{
		PlayerID: ids[50],
		Amount:   150,
	})

	assert.NilError(t, world.Tick())

	for i, id := range ids {
		wantScore := 0
		if i == 5 {
			wantScore = 105
		} else if i == 10 {
			wantScore = 110
		} else if i == 50 {
			wantScore = 150
		}
		s, err := alphaScore.Get(world, id)
		assert.NilError(t, err)
		assert.Equal(t, wantScore, s.Score)
	}
}

// TestAddToQueueDuringTickDoesNotTimeout verifies that we can add a transaction to the transaction
// queue during a game tick, and the call does not block.
func TestAddToQueueDuringTickDoesNotTimeout(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)

	modScore := ecs.NewTransactionType[*ModifyScoreTx]()
	assert.NilError(t, world.RegisterTransactions(modScore))

	inSystemCh := make(chan struct{})
	// This system will block forever. This will give us a never-ending game tick that we can use
	// to verify that the addition of more transactions doesn't block.
	world.AddSystem(func(*ecs.World, *ecs.TransactionQueue) error {
		<-inSystemCh
		select {}
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	modScore.AddToQueue(world, &ModifyScoreTx{})

	// Start a tick in the background.
	go func() {
		assert.Check(t, nil == world.Tick())
	}()
	// Make sure we're actually in the System. It will now block forever.
	inSystemCh <- struct{}{}

	// Make sure we can call AddToQueue again in a reasonable amount of time
	timeout := time.After(500 * time.Millisecond)
	doneWithAddToQueue := make(chan struct{})
	go func() {
		modScore.AddToQueue(world, &ModifyScoreTx{})
		doneWithAddToQueue <- struct{}{}
	}()

	select {
	case <-doneWithAddToQueue:
	// happy path
	case <-timeout:
		t.Fatal("timeout while trying to AddToQueue")
	}
}

// TestTransactionsAreExecutedAtNextTick verifies that while a game tick is taking place, new transactions
// are added to some queue that is not processed until the NEXT tick.
func TestTransactionsAreExecutedAtNextTick(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	modScoreTx := ecs.NewTransactionType[*ModifyScoreTx]()
	assert.NilError(t, world.RegisterTransactions(modScoreTx))

	modScoreCountCh := make(chan int)

	// Create two system that report how many instances of the ModifyScoreTx exist in the
	// transaction queue. These counts should be the same for each tick.
	world.AddSystem(func(_ *ecs.World, queue *ecs.TransactionQueue) error {
		modScores := modScoreTx.In(queue)
		modScoreCountCh <- len(modScores)
		return nil
	})

	world.AddSystem(func(_ *ecs.World, queue *ecs.TransactionQueue) error {
		modScores := modScoreTx.In(queue)
		modScoreCountCh <- len(modScores)
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	modScoreTx.AddToQueue(world, &ModifyScoreTx{})

	// Start the game tick. It will be blocked until we read from modScoreCountCh two times
	go func() {
		assert.Check(t, nil == world.Tick())
	}()

	// In the first system, we should see 1 modify score transaction
	count := <-modScoreCountCh
	assert.Equal(t, 1, count)

	// Add a transaction mid-tick.
	modScoreTx.AddToQueue(world, &ModifyScoreTx{})

	// The tick is still not over, so we should still only see 1 modify score transaction
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)

	// The tick is over. Tick again, we should see 1 tick for both systems again. This transaction
	// was added in the middle of the last tick.
	go func() {
		assert.Check(t, nil == world.Tick())
	}()
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)

	// In this final tick, we should see no modify score transactions
	go func() {
		assert.Check(t, nil == world.Tick())
	}()
	count = <-modScoreCountCh
	assert.Equal(t, 0, count)
	count = <-modScoreCountCh
	assert.Equal(t, 0, count)
}

// TestIdenticallyTypedTransactionCanBeDistinguished verifies that two transactions of the same type
// can be distinguished if they were added with different TransactionType[T]s
func TestIdenticallyTypedTransactionCanBeDistinguished(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type NewOwner struct {
		Name string
	}

	alpha := ecs.NewTransactionType[NewOwner]()
	beta := ecs.NewTransactionType[NewOwner]()
	assert.NilError(t, world.RegisterTransactions(alpha, beta))

	alpha.AddToQueue(world, NewOwner{"alpha"})
	beta.AddToQueue(world, NewOwner{"beta"})

	world.AddSystem(func(_ *ecs.World, queue *ecs.TransactionQueue) error {
		newNames := alpha.In(queue)
		assert.Check(t, 1 == len(newNames), "expected 1 transaction, not %d", len(newNames))
		assert.Check(t, "alpha" == newNames[0].Name)

		newNames = beta.In(queue)
		assert.Check(t, 1 == len(newNames), "expected 1 transaction, not %d", len(newNames))
		assert.Check(t, "beta" == newNames[0].Name)
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	assert.NilError(t, world.Tick())
}

func TestCannotRegisterDuplicateTransaction(t *testing.T) {
	tx := ecs.NewTransactionType[ModifyScoreTx]()
	world := inmem.NewECSWorldForTest(t)
	assert.Check(t, nil != world.RegisterTransactions(tx, tx))
}

func TestCannotCallRegisterTransactionsMultipleTimes(t *testing.T) {
	tx := ecs.NewTransactionType[ModifyScoreTx]()
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.RegisterTransactions(tx))
	assert.Check(t, nil != world.RegisterTransactions(tx))
}
