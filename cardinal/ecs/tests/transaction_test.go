package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

type ScoreComponent struct {
	Score int
}

type ModifyScoreTx struct {
	PlayerID storage.EntityID
	Amount   int
}

type EmptyTxResult struct{}

func TestCanQueueTransactions(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)

	// Create an entity with a score component
	score := ecs.NewComponentType[*ScoreComponent]()
	assert.NilError(t, world.RegisterComponents(score))
	modifyScoreTx := ecs.NewTransactionType[*ModifyScoreTx, *EmptyTxResult]("modify_score")
	assert.NilError(t, world.RegisterTransactions(modifyScoreTx))

	id, err := world.Create(score)
	assert.NilError(t, err)

	// Set up a system that allows for the modification of a player's score
	world.AddSystem(func(w *ecs.World, queue *ecs.TransactionQueue) error {
		modifyScore := modifyScoreTx.In(queue)
		for _, txData := range modifyScore {
			ms := txData.Value
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
	assert.NilError(t, world.Tick(context.Background()))

	// Verify the score was updated
	s, err = score.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, s.Score)

	// Tick again, but no new modifyScoreTx was added to the queue
	assert.NilError(t, world.Tick(context.Background()))

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
		assert.NilError(t, world.Tick(context.Background()))
	}

	c, err := count.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 10, c.Count)
}

func TestTransactionAreAppliedToSomeEntities(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	alphaScore := ecs.NewComponentType[ScoreComponent]()
	assert.NilError(t, world.RegisterComponents(alphaScore))

	modifyScoreTx := ecs.NewTransactionType[*ModifyScoreTx, *EmptyTxResult]("modify_score")
	assert.NilError(t, world.RegisterTransactions(modifyScoreTx))

	world.AddSystem(func(w *ecs.World, queue *ecs.TransactionQueue) error {
		modifyScores := modifyScoreTx.In(queue)
		for _, msData := range modifyScores {
			ms := msData.Value
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

	assert.NilError(t, world.Tick(context.Background()))

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

	modScore := ecs.NewTransactionType[*ModifyScoreTx, *EmptyTxResult]("modify_Score")
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
		assert.Check(t, nil == world.Tick(context.Background()))
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
	modScoreTx := ecs.NewTransactionType[*ModifyScoreTx, *EmptyTxResult]("modify_score")
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
		assert.Check(t, nil == world.Tick(context.Background()))
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
		assert.Check(t, nil == world.Tick(context.Background()))
	}()
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)

	// In this final tick, we should see no modify score transactions
	go func() {
		assert.Check(t, nil == world.Tick(context.Background()))
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

	alpha := ecs.NewTransactionType[NewOwner, EmptyTxResult]("alpha_tx")
	beta := ecs.NewTransactionType[NewOwner, EmptyTxResult]("beta_tx")
	assert.NilError(t, world.RegisterTransactions(alpha, beta))

	alpha.AddToQueue(world, NewOwner{"alpha"})
	beta.AddToQueue(world, NewOwner{"beta"})

	world.AddSystem(func(_ *ecs.World, queue *ecs.TransactionQueue) error {
		newNames := alpha.In(queue)
		assert.Check(t, 1 == len(newNames), "expected 1 transaction, not %d", len(newNames))
		assert.Check(t, "alpha" == newNames[0].Value.Name)

		newNames = beta.In(queue)
		assert.Check(t, 1 == len(newNames), "expected 1 transaction, not %d", len(newNames))
		assert.Check(t, "beta" == newNames[0].Value.Name)
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	assert.NilError(t, world.Tick(context.Background()))
}

func TestCannotRegisterDuplicateTransaction(t *testing.T) {
	tx := ecs.NewTransactionType[ModifyScoreTx, EmptyTxResult]("modify_score")
	world := inmem.NewECSWorldForTest(t)
	assert.Check(t, nil != world.RegisterTransactions(tx, tx))
}

func TestCannotCallRegisterTransactionsMultipleTimes(t *testing.T) {
	tx := ecs.NewTransactionType[ModifyScoreTx, EmptyTxResult]("modify_score")
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.RegisterTransactions(tx))
	assert.Check(t, nil != world.RegisterTransactions(tx))
}

func TestCanDecodeEVMTransactions(t *testing.T) {
	// the tx we are going to test against
	type FooTx struct {
		X, Y uint64
		Name string
	}

	// create the EVM binding. this bit can be code generated by Beam :D
	FooEvmTx, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "X", Type: "uint64"},
		{Name: "Y", Type: "uint64"},
		{Name: "Name", Type: "string"},
	})
	assert.NilError(t, err)
	FooEvmTx.TupleType = reflect.TypeOf(FooTx{})

	// now we get the ABI encoded version of the struct. this gives us the equivalent of
	// calling abi.Encode on a solidity struct with the same types/fields.
	args := abi.Arguments{{Type: FooEvmTx}}
	tx := FooTx{1, 2, "foo"}
	bz, err := args.Pack(tx)
	assert.NilError(t, err)

	// set up the ITransaction.
	itx := ecs.NewTransactionType[FooTx, EmptyTxResult]("FooTx")
	itx.SetEVMType(&FooEvmTx)

	// decode the evm bytes
	fooTx, err := itx.DecodeEVMBytes(bz)
	assert.NilError(t, err)

	// we should be able to cast back to our concrete Go struct.
	f, ok := fooTx.(FooTx)
	assert.Equal(t, ok, true)
	assert.DeepEqual(t, f, tx)
}

func TestCannotDecodeEVMBeforeSetEVM(t *testing.T) {
	type foo struct{}
	tx := ecs.NewTransactionType[foo, EmptyTxResult]("foo")
	_, err := tx.DecodeEVMBytes([]byte{})
	assert.ErrorContains(t, err, "cannot call DecodeEVMBytes without setting via SetEVMType first")
}

func TestCannotHaveDuplicateTransactionNames(t *testing.T) {
	type SomeTx struct {
		X, Y, Z int
	}
	type OtherTx struct {
		Alpha, Beta string
	}
	world := inmem.NewECSWorldForTest(t)
	alphaTx := ecs.NewTransactionType[SomeTx, EmptyTxResult]("name_match")
	betaTx := ecs.NewTransactionType[OtherTx, EmptyTxResult]("name_match")
	assert.ErrorIs(t, world.RegisterTransactions(alphaTx, betaTx), ecs.ErrorDuplicateTransactionName)
}

func TestCanGetTransactionErrorsAndResults(t *testing.T) {
	type MoveTx struct {
		DeltaX, DeltaY int
	}
	type MoveTxResult struct {
		EndX, EndY int
	}
	world := inmem.NewECSWorldForTest(t)

	// Each transaction now needs an input and an output
	moveTx := ecs.NewTransactionType[MoveTx, MoveTxResult]("move")
	assert.NilError(t, world.RegisterTransactions(moveTx))

	wantFirstError := errors.New("this is a transaction error")
	wantSecondError := errors.New("another transaction error")
	wantDeltaX, wantDeltaY := 99, 100

	world.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		// This new TxsIn function returns a triplet of information:
		// 1) The transaction input
		// 2) An ID that uniquely identifies this specific transaction
		// 3) The signature
		// This function would replace both "In" and "TxsAndSigsIn"
		txs := moveTx.In(queue)
		assert.Equal(t, 1, len(txs), "expected 1 move transaction")
		tx := txs[0]
		// The input for the transaction is found at tx.Val
		assert.Equal(t, wantDeltaX, tx.Value.DeltaX)
		assert.Equal(t, wantDeltaY, tx.Value.DeltaY)

		// AddError will associate an error with the tx.ID. Multiple errors can be
		// associated with a transaction.
		moveTx.AddError(world, tx.ID, wantFirstError)
		moveTx.AddError(world, tx.ID, wantSecondError)

		// SetResult sets the output for the transaction. Only one output can be set
		// for a tx.ID (the last assigned result will clobber other results)
		moveTx.SetResult(world, tx.ID, MoveTxResult{42, 42})
		return nil
	})
	assert.NilError(t, world.LoadGameState())
	_ = moveTx.AddToQueue(world, MoveTx{99, 100})

	// Tick the game so the transaction is processed
	assert.NilError(t, world.Tick(context.Background()))

	tick := world.CurrentTick() - 1
	receipts, err := world.GetTransactionReceiptsForTick(tick)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	r := receipts[0]
	assert.Equal(t, 2, len(r.Errs))
	assert.ErrorIs(t, wantFirstError, r.Errs[0])
	assert.ErrorIs(t, wantSecondError, r.Errs[1])
	got, ok := r.Result.(MoveTxResult)
	assert.Check(t, ok)
	assert.Equal(t, MoveTxResult{42, 42}, got)
}

func TestSystemCanFindErrorsFromEarlierSystem(t *testing.T) {
	type TxIn struct {
		Number int
	}
	type TxOut struct {
		Number int
	}
	world := inmem.NewECSWorldForTest(t)
	numTx := ecs.NewTransactionType[TxIn, TxOut]("number")
	world.RegisterTransactions(numTx)
	wantErr := errors.New("some transaction error")
	systemCalls := 0
	world.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		systemCalls++
		txs := numTx.In(queue)
		assert.Equal(t, 1, len(txs))
		id := txs[0].ID
		_, _, ok := numTx.GetReceipt(world, id)
		assert.Check(t, !ok)
		numTx.AddError(world, id, wantErr)
		return nil
	})

	world.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		systemCalls++
		txs := numTx.In(queue)
		assert.Equal(t, 1, len(txs))
		id := txs[0].ID
		_, errs, ok := numTx.GetReceipt(world, id)
		assert.Check(t, ok)
		assert.Equal(t, 1, len(errs))
		assert.ErrorIs(t, wantErr, errs[0])
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	_ = numTx.AddToQueue(world, TxIn{100})

	assert.NilError(t, world.Tick(context.Background()))
	assert.Equal(t, 2, systemCalls)
}

func TestSystemCanClobberTransactionResult(t *testing.T) {
	type TxIn struct {
		Number int
	}
	type TxOut struct {
		Number int
	}
	world := inmem.NewECSWorldForTest(t)
	numTx := ecs.NewTransactionType[TxIn, TxOut]("number")
	world.RegisterTransactions(numTx)
	systemCalls := 0

	firstResult := TxOut{1234}
	secondResult := TxOut{5678}
	world.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		systemCalls++
		txs := numTx.In(queue)
		assert.Equal(t, 1, len(txs))
		id := txs[0].ID
		_, _, ok := numTx.GetReceipt(world, id)
		assert.Check(t, !ok)
		numTx.SetResult(world, id, firstResult)
		return nil
	})

	world.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		systemCalls++
		txs := numTx.In(queue)
		assert.Equal(t, 1, len(txs))
		id := txs[0].ID
		out, errs, ok := numTx.GetReceipt(world, id)
		assert.Check(t, ok)
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, TxOut{1234}, out)
		numTx.SetResult(world, id, secondResult)
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	_ = numTx.AddToQueue(world, TxIn{100})

	assert.NilError(t, world.Tick(context.Background()))

	prevTick := world.CurrentTick() - 1
	receipts, err := world.GetTransactionReceiptsForTick(prevTick)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	r := receipts[0]
	assert.Equal(t, 0, len(r.Errs))
	gotResult, ok := r.Result.(TxOut)
	assert.Check(t, ok)
	assert.Equal(t, secondResult, gotResult)
}
