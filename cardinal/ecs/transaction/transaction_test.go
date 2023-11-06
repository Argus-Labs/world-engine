package transaction_test

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

type ScoreComponent struct {
	Score int
}

func (ScoreComponent) Name() string {
	return "score"
}

type ModifyScoreTx struct {
	PlayerID entity.ID
	Amount   int
}

type EmptyTxResult struct{}

func TestReadTypeNotStructs(t *testing.T) {
	defer func() {
		// test should trigger a panic. it is swallowed here.
		panicValue := recover()
		assert.Assert(t, panicValue != nil)

		defer func() {
			// deferred function should not fail
			panicValue = recover()
			assert.Assert(t, panicValue == nil)
		}()

		ecs.NewTransactionType[*ModifyScoreTx, *EmptyTxResult]("modify_score2")
	}()
	ecs.NewTransactionType[string, string]("modify_score1")
}

func TestCanQueueTransactions(t *testing.T) {
	world := ecs.NewTestWorld(t)

	// Create an entity with a score component
	assert.NilError(t, ecs.RegisterComponent[ScoreComponent](world))
	modifyScoreTx := ecs.NewTransactionType[*ModifyScoreTx, *EmptyTxResult]("modify_score")
	assert.NilError(t, world.RegisterTransactions(modifyScoreTx))

	wCtx := ecs.NewWorldContext(world)
	id, err := component.Create(wCtx, ScoreComponent{})
	assert.NilError(t, err)

	// Set up a system that allows for the modification of a player's score
	world.AddSystem(func(wCtx ecs.WorldContext) error {
		modifyScore := modifyScoreTx.In(wCtx)
		for _, txData := range modifyScore {
			ms := txData.Value
			err = component.UpdateComponent[ScoreComponent](wCtx, ms.PlayerID, func(s *ScoreComponent) *ScoreComponent {
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

	assert.NilError(t, component.SetComponent[ScoreComponent](wCtx, id, &ScoreComponent{}))

	// Verify the score is 0
	s, err := component.GetComponent[ScoreComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 0, s.Score)

	// Process a game tick
	assert.NilError(t, world.Tick(context.Background()))

	// Verify the score was updated
	s, err = component.GetComponent[ScoreComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, s.Score)

	// Tick again, but no new modifyScoreTx was added to the queue
	assert.NilError(t, world.Tick(context.Background()))

	// Verify the score hasn't changed
	s, err = component.GetComponent[ScoreComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, s.Score)
}

type CounterComponent struct {
	Count int
}

func (CounterComponent) Name() string {
	return "count"
}

func TestSystemsAreExecutedDuringGameTick(t *testing.T) {
	world := ecs.NewTestWorld(t)

	assert.NilError(t, ecs.RegisterComponent[CounterComponent](world))

	wCtx := ecs.NewWorldContext(world)
	id, err := component.Create(wCtx, CounterComponent{})
	assert.NilError(t, err)
	world.AddSystem(func(wCtx ecs.WorldContext) error {
		return component.UpdateComponent[CounterComponent](wCtx, id, func(c *CounterComponent) *CounterComponent {
			c.Count++
			return c
		})
	})
	assert.NilError(t, world.LoadGameState())

	for i := 0; i < 10; i++ {
		assert.NilError(t, world.Tick(context.Background()))
	}

	c, err := component.GetComponent[CounterComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 10, c.Count)
}

func TestTransactionAreAppliedToSomeEntities(t *testing.T) {
	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[ScoreComponent](world))

	modifyScoreTx := ecs.NewTransactionType[*ModifyScoreTx, *EmptyTxResult]("modify_score")
	assert.NilError(t, world.RegisterTransactions(modifyScoreTx))

	world.AddSystem(func(wCtx ecs.WorldContext) error {
		modifyScores := modifyScoreTx.In(wCtx)
		for _, msData := range modifyScores {
			ms := msData.Value
			err := component.UpdateComponent[ScoreComponent](wCtx, ms.PlayerID, func(s *ScoreComponent) *ScoreComponent {
				s.Score += ms.Amount
				return s
			})
			assert.Check(t, err == nil)
		}
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	wCtx := ecs.NewWorldContext(world)
	ids, err := component.CreateMany(wCtx, 100, ScoreComponent{})
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
		switch i {
		case 5:
			wantScore = 105
		case 10:
			wantScore = 110
		case 50:
			wantScore = 150
		}
		s, err := component.GetComponent[ScoreComponent](wCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, wantScore, s.Score)
	}
}

// TestAddToQueueDuringTickDoesNotTimeout verifies that we can add a transaction to the transaction
// queue during a game tick, and the call does not block.
func TestAddToQueueDuringTickDoesNotTimeout(t *testing.T) {
	world := ecs.NewTestWorld(t)

	modScore := ecs.NewTransactionType[*ModifyScoreTx, *EmptyTxResult]("modify_Score")
	assert.NilError(t, world.RegisterTransactions(modScore))

	inSystemCh := make(chan struct{})
	// This system will block forever. This will give us a never-ending game tick that we can use
	// to verify that the addition of more transactions doesn't block.
	world.AddSystem(func(ecs.WorldContext) error {
		<-inSystemCh
		select {}
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
	world := ecs.NewTestWorld(t)
	modScoreTx := ecs.NewTransactionType[*ModifyScoreTx, *EmptyTxResult]("modify_score")
	assert.NilError(t, world.RegisterTransactions(modScoreTx))
	ctx := context.Background()
	tickStart := make(chan time.Time)
	tickDone := make(chan uint64)
	world.StartGameLoop(ctx, tickStart, tickDone)

	modScoreCountCh := make(chan int)

	// Create two system that report how many instances of the ModifyScoreTx exist in the
	// transaction queue. These counts should be the same for each tick. modScoreCountCh is an unbuffered channel
	// so these systems will block while writing to modScoreCountCh. This allows the test to ensure that we can run
	// commands mid-tick.
	world.AddSystem(func(wCtx ecs.WorldContext) error {
		modScores := modScoreTx.In(wCtx)
		modScoreCountCh <- len(modScores)
		return nil
	})

	world.AddSystem(func(wCtx ecs.WorldContext) error {
		modScores := modScoreTx.In(wCtx)
		modScoreCountCh <- len(modScores)
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	modScoreTx.AddToQueue(world, &ModifyScoreTx{})

	// Start the game tick. The tick will block while waiting to write to modScoreCountCh
	tickStart <- time.Now()

	// In the first system, we should see 1 modify score transaction
	count := <-modScoreCountCh
	assert.Equal(t, 1, count)

	// Add two transactions mid-tick.
	modScoreTx.AddToQueue(world, &ModifyScoreTx{})
	modScoreTx.AddToQueue(world, &ModifyScoreTx{})

	// The tick is still not over, so we should still only see 1 modify score transaction
	count = <-modScoreCountCh
	assert.Equal(t, 1, count)

	// Block until the tick has completed.
	<-tickDone

	// Start the next tick.
	tickStart <- time.Now()

	// This second tick shold find 2 ModifyScore transactions. They were added in the middle of the previous tick.
	count = <-modScoreCountCh
	assert.Equal(t, 2, count)
	count = <-modScoreCountCh
	assert.Equal(t, 2, count)

	// Block until the tick has completed.
	<-tickDone

	// In this final tick, we should see no modify score transactions
	tickStart <- time.Now()
	count = <-modScoreCountCh
	assert.Equal(t, 0, count)
	count = <-modScoreCountCh
	assert.Equal(t, 0, count)
	<-tickDone
}

// TestIdenticallyTypedTransactionCanBeDistinguished verifies that two transactions of the same type
// can be distinguished if they were added with different TransactionType[T]s.
func TestIdenticallyTypedTransactionCanBeDistinguished(t *testing.T) {
	world := ecs.NewTestWorld(t)
	type NewOwner struct {
		Name string
	}

	alpha := ecs.NewTransactionType[NewOwner, EmptyTxResult]("alpha_tx")
	beta := ecs.NewTransactionType[NewOwner, EmptyTxResult]("beta_tx")
	assert.NilError(t, world.RegisterTransactions(alpha, beta))

	alpha.AddToQueue(world, NewOwner{"alpha"})
	beta.AddToQueue(world, NewOwner{"beta"})

	world.AddSystem(func(wCtx ecs.WorldContext) error {
		newNames := alpha.In(wCtx)
		assert.Check(t, len(newNames) == 1, "expected 1 transaction, not %d", len(newNames))
		assert.Check(t, newNames[0].Value.Name == "alpha")

		newNames = beta.In(wCtx)
		assert.Check(t, len(newNames) == 1, "expected 1 transaction, not %d", len(newNames))
		assert.Check(t, newNames[0].Value.Name == "beta")
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	assert.NilError(t, world.Tick(context.Background()))
}

func TestCannotRegisterDuplicateTransaction(t *testing.T) {
	tx := ecs.NewTransactionType[ModifyScoreTx, EmptyTxResult]("modify_score")
	world := ecs.NewTestWorld(t)
	assert.Check(t, nil != world.RegisterTransactions(tx, tx))
}

func TestCannotCallRegisterTransactionsMultipleTimes(t *testing.T) {
	tx := ecs.NewTransactionType[ModifyScoreTx, EmptyTxResult]("modify_score")
	world := ecs.NewTestWorld(t)
	assert.NilError(t, world.RegisterTransactions(tx))
	assert.Check(t, nil != world.RegisterTransactions(tx))
}

func TestCanEncodeDecodeEVMTransactions(t *testing.T) {
	// the tx we are going to test against
	type FooTx struct {
		X, Y uint64
		Name string
	}

	tx := FooTx{1, 2, "foo"}
	// set up the ITransaction.
	itx := ecs.NewTransactionType[FooTx, EmptyTxResult]("FooTx", ecs.WithTxEVMSupport[FooTx, EmptyTxResult])
	bz, err := itx.ABIEncode(tx)
	assert.NilError(t, err)

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
	assert.ErrorIs(t, err, ecs.ErrEVMTypeNotSet)
}

func TestCannotHaveDuplicateTransactionNames(t *testing.T) {
	type SomeTx struct {
		X, Y, Z int
	}
	type OtherTx struct {
		Alpha, Beta string
	}
	world := ecs.NewTestWorld(t)
	alphaTx := ecs.NewTransactionType[SomeTx, EmptyTxResult]("name_match")
	betaTx := ecs.NewTransactionType[OtherTx, EmptyTxResult]("name_match")
	assert.ErrorIs(t, world.RegisterTransactions(alphaTx, betaTx), ecs.ErrDuplicateTransactionName)
}

func TestCanGetTransactionErrorsAndResults(t *testing.T) {
	type MoveTx struct {
		DeltaX, DeltaY int
	}
	type MoveTxResult struct {
		EndX, EndY int
	}
	world := ecs.NewTestWorld(t)

	// Each transaction now needs an input and an output
	moveTx := ecs.NewTransactionType[MoveTx, MoveTxResult]("move")
	assert.NilError(t, world.RegisterTransactions(moveTx))

	wantFirstError := errors.New("this is a transaction error")
	wantSecondError := errors.New("another transaction error")
	wantDeltaX, wantDeltaY := 99, 100

	world.AddSystem(func(wCtx ecs.WorldContext) error {
		// This new TxsIn function returns a triplet of information:
		// 1) The transaction input
		// 2) An ID that uniquely identifies this specific transaction
		// 3) The signature
		// This function would replace both "In" and "TxsAndSigsIn"
		txs := moveTx.In(wCtx)
		assert.Equal(t, 1, len(txs), "expected 1 move transaction")
		tx := txs[0]
		// The input for the transaction is found at tx.Val
		assert.Equal(t, wantDeltaX, tx.Value.DeltaX)
		assert.Equal(t, wantDeltaY, tx.Value.DeltaY)

		// AddError will associate an error with the tx.TxHash. Multiple errors can be
		// associated with a transaction.
		moveTx.AddError(wCtx, tx.TxHash, wantFirstError)
		moveTx.AddError(wCtx, tx.TxHash, wantSecondError)

		// SetResult sets the output for the transaction. Only one output can be set
		// for a tx.TxHash (the last assigned result will clobber other results)
		moveTx.SetResult(wCtx, tx.TxHash, MoveTxResult{42, 42})
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
	world := ecs.NewTestWorld(t)
	numTx := ecs.NewTransactionType[TxIn, TxOut]("number")
	assert.NilError(t, world.RegisterTransactions(numTx))
	wantErr := errors.New("some transaction error")
	systemCalls := 0
	world.AddSystem(func(wCtx ecs.WorldContext) error {
		systemCalls++
		txs := numTx.In(wCtx)
		assert.Equal(t, 1, len(txs))
		hash := txs[0].TxHash
		_, _, ok := numTx.GetReceipt(wCtx, hash)
		assert.Check(t, !ok)
		numTx.AddError(wCtx, hash, wantErr)
		return nil
	})

	world.AddSystem(func(wCtx ecs.WorldContext) error {
		systemCalls++
		txs := numTx.In(wCtx)
		assert.Equal(t, 1, len(txs))
		hash := txs[0].TxHash
		_, errs, ok := numTx.GetReceipt(wCtx, hash)
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
	world := ecs.NewTestWorld(t)
	numTx := ecs.NewTransactionType[TxIn, TxOut]("number")
	assert.NilError(t, world.RegisterTransactions(numTx))
	systemCalls := 0

	firstResult := TxOut{1234}
	secondResult := TxOut{5678}
	world.AddSystem(func(wCtx ecs.WorldContext) error {
		systemCalls++
		txs := numTx.In(wCtx)
		assert.Equal(t, 1, len(txs))
		hash := txs[0].TxHash
		_, _, ok := numTx.GetReceipt(wCtx, hash)
		assert.Check(t, !ok)
		numTx.SetResult(wCtx, hash, firstResult)
		return nil
	})

	world.AddSystem(func(wCtx ecs.WorldContext) error {
		systemCalls++
		txs := numTx.In(wCtx)
		assert.Equal(t, 1, len(txs))
		hash := txs[0].TxHash
		out, errs, ok := numTx.GetReceipt(wCtx, hash)
		assert.Check(t, ok)
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, TxOut{1234}, out)
		numTx.SetResult(wCtx, hash, secondResult)
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

func TestCopyTransactions(t *testing.T) {
	type FooTx struct {
		X int
	}
	txq := transaction.NewTxQueue()
	txq.AddTransaction(1, FooTx{X: 3}, &sign.Transaction{PersonaTag: "foo"})
	txq.AddTransaction(2, FooTx{X: 4}, &sign.Transaction{PersonaTag: "bar"})

	copyTxq := txq.CopyTransactions()
	assert.Equal(t, copyTxq.GetAmountOfTxs(), 2)
	assert.Equal(t, txq.GetAmountOfTxs(), 0)
}

func TestNewTransactionPanicsIfNoName(t *testing.T) {
	type Foo struct{}
	require.Panics(t, func() {
		ecs.NewTransactionType[Foo, Foo]("")
	})
}
