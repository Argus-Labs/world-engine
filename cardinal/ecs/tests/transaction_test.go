package tests

import (
	"strings"
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
	score := ecs.NewComponentType[ScoreComponent]()
	world.RegisterComponents(score)
	id, err := world.Create(score)
	assert.NilError(t, err)
	modifyScoreTx := ecs.NewTransactionType[ModifyScoreTx](world, "modifyScore")

	// Set up a system that allows for the modification of a player's score
	world.AddSystem(func(queue *ecs.TransactionQueue) {
		modifyScore := modifyScoreTx.In(queue)
		for _, ms := range modifyScore {
			err := score.Update(ms.PlayerID, func(s *ScoreComponent) {
				s.Score += ms.Amount
			})
			assert.Check(t, err == nil)
		}
	})

	modifyScoreTx.AddToQueue(&ModifyScoreTx{id, 100})

	// Verify the score is 0
	s, err := score.Get(id)
	assert.NilError(t, err)
	assert.Equal(t, 0, s.Score)

	// Process a game tick
	world.Tick()

	// Verify the score was updated
	s, err = score.Get(id)
	assert.NilError(t, err)
	assert.Equal(t, 100, s.Score)

	// Tick again, but no new modifyScoreTx was added to the queue
	world.Tick()

	// Verify the score hasn't changed
	s, err = score.Get(id)
	assert.NilError(t, err)
	assert.Equal(t, 100, s.Score)
}

func TestSystemsAreExecutedDuringGameTick(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type CounterComponent struct {
		Count int
	}
	count := ecs.NewComponentType[CounterComponent]()
	world.RegisterComponents(count)

	id, err := world.Create(count)
	assert.NilError(t, err)
	world.AddSystem(func(*ecs.TransactionQueue) {
		count.Update(id, func(c *CounterComponent) {
			c.Count++
		})
	})

	for i := 0; i < 10; i++ {
		world.Tick()
	}

	c, err := count.Get(id)
	assert.NilError(t, err)
	assert.Equal(t, 10, c.Count)
}

func TestTransactionAreAppliedToSomeEntities(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	alphaScore := ecs.NewComponentType[ScoreComponent]()
	world.RegisterComponents(alphaScore)

	modifyScoreTx := ecs.NewTransactionType[ModifyScoreTx](world, "modifyScore")

	world.AddSystem(func(queue *ecs.TransactionQueue) {
		modifyScores := modifyScoreTx.In(queue)
		for _, ms := range modifyScores {
			err := alphaScore.Update(ms.PlayerID, func(s *ScoreComponent) {
				s.Score += ms.Amount
			})
			assert.Check(t, err == nil)
		}
	})

	ids, err := world.CreateMany(100, alphaScore)
	assert.NilError(t, err)
	// Entities at index 5, 10 and 50 will be updated with some values
	modifyScoreTx.AddToQueue(&ModifyScoreTx{
		PlayerID: ids[5],
		Amount:   105,
	})
	modifyScoreTx.AddToQueue(&ModifyScoreTx{
		PlayerID: ids[10],
		Amount:   110,
	})
	modifyScoreTx.AddToQueue(&ModifyScoreTx{
		PlayerID: ids[50],
		Amount:   150,
	})

	world.Tick()

	for i, id := range ids {
		wantScore := 0
		if i == 5 {
			wantScore = 105
		} else if i == 10 {
			wantScore = 110
		} else if i == 50 {
			wantScore = 150
		}
		s, err := alphaScore.Get(id)
		assert.NilError(t, err)
		assert.Equal(t, wantScore, s.Score)
	}
}

// TestAddToQueueDuringTickDoesNotTimeout verifies that we can add a transaction to the transaction
// queue during a game tick, and the call does not block.
func TestAddToQueueDuringTickDoesNotTimeout(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)

	// "modify_score" will block forever. This will give us a never-ending game tick that we can use
	// to verify the addition of more user transactions don't block.
	modScore := ecs.NewTransactionType[ModifyScoreTx](world, "modifyScore")

	inSystemCh := make(chan struct{})
	// This system will block forever. This will give us a never-ending game tick that we can use
	// to verify the addition of more transactions doesn't block.
	world.AddSystem(func(*ecs.TransactionQueue) {
		<-inSystemCh
		select {}
	})

	modScore.AddToQueue(&ModifyScoreTx{})

	// Start a tick in the background.
	go world.Tick()
	// Make sure we're actually in the System. It will now block forever.
	inSystemCh <- struct{}{}

	// Make sure we can call AddToQueue again in a reasonable amount of time
	timeout := time.After(500 * time.Millisecond)
	doneWithAddToQueue := make(chan struct{})
	go func() {
		modScore.AddToQueue(&ModifyScoreTx{})
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
	modScoreTx := ecs.NewTransactionType[ModifyScoreTx](world, "modifyScore")

	modScoreCountCh := make(chan int)

	// Create two system that report how many instances of the ModifyScoreTx exist in the
	// transaction queue. These counts should be the same for each tick.
	world.AddSystem(func(queue *ecs.TransactionQueue) {
		modScores := modScoreTx.In(queue)
		modScoreCountCh <- len(modScores)
	})

	world.AddSystem(func(queue *ecs.TransactionQueue) {
		modScores := modScoreTx.In(queue)
		modScoreCountCh <- len(modScores)
	})

	modScoreTx.AddToQueue(&ModifyScoreTx{})

	// Star the game tick. It will be blocked until we read from modScoreCountCh two times
	go world.Tick()

	// In the first system, we should see 1 modify score transaction
	count := <-modScoreCountCh
	assert.Equal(t, 1, count)

	modScoreTx.AddToQueue(&ModifyScoreTx{})

	// The tick is still not over, so we should still only see 1 modify score transaction
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)

	// The tick is over. Tick again, we should see 1 tick for both systems again
	go world.Tick()
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)

	// In this final tick, we should see no modify score transactions
	go world.Tick()
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

	alpha := ecs.NewTransactionType[NewOwner](world, "alpha")
	beta := ecs.NewTransactionType[NewOwner](world, "beta")

	alpha.AddToQueue(&NewOwner{"alpha"})
	beta.AddToQueue(&NewOwner{"beta"})

	world.AddSystem(func(queue *ecs.TransactionQueue) {
		newNames := alpha.In(queue)
		assert.Check(t, 1 == len(newNames), "expected 1 transaction, not %d", len(newNames))
		assert.Check(t, "alpha" == newNames[0].Name)

		newNames = beta.In(queue)
		assert.Check(t, 1 == len(newNames), "expected 1 transaction, not %d", len(newNames))
		assert.Check(t, "beta" == newNames[0].Name)
	})

	world.Tick()
}

func TestCannotCreateMultipleTransactionTypesWithTheSameName(t *testing.T) {
	defer func() {
		err := recover().(string)
		assert.Check(t, strings.Contains(err, "Multiple definitions of transaction"))
	}()
	world := inmem.NewECSWorldForTest(t)
	ecs.NewTransactionType[ModifyScoreTx](world, "same_name")
	ecs.NewTransactionType[ModifyScoreTx](world, "same_name")
}
