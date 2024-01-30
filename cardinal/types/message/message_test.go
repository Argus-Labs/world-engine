package message_test

import (
	"context"
	"errors"
	"pkg.world.dev/world-engine/cardinal/ecs/messages"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/txpool"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/sign"
)

type ScoreComponent struct {
	Score int
}

func (ScoreComponent) Name() string {
	return "score"
}

type ModifyScoreMsg struct {
	PlayerID entity.ID
	Amount   int
}

type EmptyMsgResult struct{}

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

		ecs.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_score2")
	}()
	ecs.NewMessageType[string, string]("modify_score1")
}

func TestCanQueueTransactions(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine

	// Create an entity with a score component
	assert.NilError(t, ecs.RegisterComponent[ScoreComponent](eng))
	modifyScoreMsg := ecs.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_score")
	assert.NilError(t, eng.RegisterMessages(modifyScoreMsg))

	eCtx := ecs.NewEngineContext(eng)

	// Set up a system that allows for the modification of a player's score
	err := eng.RegisterSystems(
		func(eCtx engine.Context) error {
			modifyScore := modifyScoreMsg.In(eCtx)
			for _, txData := range modifyScore {
				ms := txData.Msg
				err := ecs.UpdateComponent[ScoreComponent](
					eCtx, ms.PlayerID, func(s *ScoreComponent) *ScoreComponent {
						s.Score += ms.Amount
						return s
					},
				)
				if err != nil {
					return err
				}
			}
			return nil
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())
	id, err := ecs.Create(eCtx, ScoreComponent{})
	assert.NilError(t, err)

	modifyScoreMsg.AddToQueue(eng, &ModifyScoreMsg{id, 100})

	assert.NilError(t, ecs.SetComponent[ScoreComponent](eCtx, id, &ScoreComponent{}))

	// Verify the score is 0
	s, err := ecs.GetComponent[ScoreComponent](eCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 0, s.Score)

	// Process a game tick
	assert.NilError(t, eng.Tick(context.Background()))

	// Verify the score was updated
	s, err = ecs.GetComponent[ScoreComponent](eCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, s.Score)

	// Tick again, but no new modifyScoreMsg was added to the queue
	assert.NilError(t, eng.Tick(context.Background()))

	// Verify the score hasn't changed
	s, err = ecs.GetComponent[ScoreComponent](eCtx, id)
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
	eng := testutils.NewTestFixture(t, nil).Engine

	assert.NilError(t, ecs.RegisterComponent[CounterComponent](eng))

	eCtx := ecs.NewEngineContext(eng)

	err := eng.RegisterSystems(
		func(eCtx engine.Context) error {
			search := eng.NewSearch(filter.Exact(CounterComponent{}))
			id := search.MustFirst()
			return ecs.UpdateComponent[CounterComponent](
				eCtx, id, func(c *CounterComponent) *CounterComponent {
					c.Count++
					return c
				},
			)
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())
	id, err := ecs.Create(eCtx, CounterComponent{})
	assert.NilError(t, err)

	for i := 0; i < 10; i++ {
		assert.NilError(t, eng.Tick(context.Background()))
	}

	c, err := ecs.GetComponent[CounterComponent](eCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 10, c.Count)
}

func TestTransactionAreAppliedToSomeEntities(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine
	assert.NilError(t, ecs.RegisterComponent[ScoreComponent](eng))

	modifyScoreMsg := ecs.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_score")
	assert.NilError(t, eng.RegisterMessages(modifyScoreMsg))

	err := eng.RegisterSystems(
		func(eCtx engine.Context) error {
			modifyScores := modifyScoreMsg.In(eCtx)
			for _, msData := range modifyScores {
				ms := msData.Msg
				err := ecs.UpdateComponent[ScoreComponent](
					eCtx, ms.PlayerID, func(s *ScoreComponent) *ScoreComponent {
						s.Score += ms.Amount
						return s
					},
				)
				assert.Check(t, err == nil)
			}
			return nil
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())

	eCtx := ecs.NewEngineContext(eng)
	ids, err := ecs.CreateMany(eCtx, 100, ScoreComponent{})
	assert.NilError(t, err)
	// Entities at index 5, 10 and 50 will be updated with some values
	modifyScoreMsg.AddToQueue(
		eng, &ModifyScoreMsg{
			PlayerID: ids[5],
			Amount:   105,
		},
	)
	modifyScoreMsg.AddToQueue(
		eng, &ModifyScoreMsg{
			PlayerID: ids[10],
			Amount:   110,
		},
	)
	modifyScoreMsg.AddToQueue(
		eng, &ModifyScoreMsg{
			PlayerID: ids[50],
			Amount:   150,
		},
	)

	assert.NilError(t, eng.Tick(context.Background()))

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
		s, err := ecs.GetComponent[ScoreComponent](eCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, wantScore, s.Score)
	}
}

// TestAddToQueueDuringTickDoesNotTimeout verifies that we can add a transaction to the transaction
// queue during a game tick, and the call does not block.
func TestAddToQueueDuringTickDoesNotTimeout(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine

	modScore := ecs.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_Score")
	assert.NilError(t, eng.RegisterMessages(modScore))

	inSystemCh := make(chan struct{})
	// This system will block forever. This will give us a never-ending game tick that we can use
	// to verify that the addition of more transactions doesn't block.
	err := eng.RegisterSystems(
		func(engine.Context) error {
			<-inSystemCh
			select {}
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())

	modScore.AddToQueue(eng, &ModifyScoreMsg{})

	// Start a tick in the background.
	go func() {
		assert.Check(t, nil == eng.Tick(context.Background()))
	}()
	// Make sure we're actually in the System. It will now block forever.
	inSystemCh <- struct{}{}

	// Make sure we can call AddToQueue again in a reasonable amount of time
	timeout := time.After(500 * time.Millisecond)
	doneWithAddToQueue := make(chan struct{})
	go func() {
		modScore.AddToQueue(eng, &ModifyScoreMsg{})
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
	eng := testutils.NewTestFixture(t, nil).Engine
	modScoreMsg := ecs.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_score")
	assert.NilError(t, eng.RegisterMessages(modScoreMsg))
	ctx := context.Background()
	tickStart := make(chan time.Time)
	tickDone := make(chan uint64)
	eng.StartGameLoop(ctx, tickStart, tickDone)

	modScoreCountCh := make(chan int)

	// Create two system that report how many instances of the ModifyScoreMsg exist in the
	// transaction queue. These counts should be the same for each tick. modScoreCountCh is an unbuffered channel
	// so these systems will block while writing to modScoreCountCh. This allows the test to ensure that we can run
	// commands mid-tick.
	err := eng.RegisterSystems(
		func(eCtx engine.Context) error {
			modScores := modScoreMsg.In(eCtx)
			modScoreCountCh <- len(modScores)
			return nil
		},
	)
	assert.NilError(t, err)

	err = eng.RegisterSystems(
		func(eCtx engine.Context) error {
			modScores := modScoreMsg.In(eCtx)
			modScoreCountCh <- len(modScores)
			return nil
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())

	modScoreMsg.AddToQueue(eng, &ModifyScoreMsg{})

	// Start the game tick. The tick will block while waiting to write to modScoreCountCh
	tickStart <- time.Now()

	// In the first system, we should see 1 modify score transaction
	count := <-modScoreCountCh
	assert.Equal(t, 1, count)

	// Add two transactions mid-tick.
	modScoreMsg.AddToQueue(eng, &ModifyScoreMsg{})
	modScoreMsg.AddToQueue(eng, &ModifyScoreMsg{})

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
// can be distinguished if they were added with different MessageType[T]s.
func TestIdenticallyTypedTransactionCanBeDistinguished(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine
	type NewOwner struct {
		Name string
	}

	alpha := ecs.NewMessageType[NewOwner, EmptyMsgResult]("alpha_msg")
	beta := ecs.NewMessageType[NewOwner, EmptyMsgResult]("beta_msg")
	assert.NilError(t, eng.RegisterMessages(alpha, beta))

	alpha.AddToQueue(eng, NewOwner{"alpha"})
	beta.AddToQueue(eng, NewOwner{"beta"})

	err := eng.RegisterSystems(
		func(eCtx engine.Context) error {
			newNames := alpha.In(eCtx)
			assert.Check(t, len(newNames) == 1, "expected 1 transaction, not %d", len(newNames))
			assert.Check(t, newNames[0].Msg.Name == "alpha")

			newNames = beta.In(eCtx)
			assert.Check(t, len(newNames) == 1, "expected 1 transaction, not %d", len(newNames))
			assert.Check(t, newNames[0].Msg.Name == "beta")
			return nil
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())

	assert.NilError(t, eng.Tick(context.Background()))
}

func TestCannotRegisterDuplicateTransaction(t *testing.T) {
	msg := ecs.NewMessageType[ModifyScoreMsg, EmptyMsgResult]("modify_score")
	engine := testutils.NewTestFixture(t, nil).Engine
	assert.Check(t, nil != engine.RegisterMessages(msg, msg))
}

func TestCannotCallRegisterTransactionsMultipleTimes(t *testing.T) {
	msg := ecs.NewMessageType[ModifyScoreMsg, EmptyMsgResult]("modify_score")
	engine := testutils.NewTestFixture(t, nil).Engine
	assert.NilError(t, engine.RegisterMessages(msg))
	assert.Check(t, nil != engine.RegisterMessages(msg))
}

func TestCanEncodeDecodeEVMTransactions(t *testing.T) {
	// the msg we are going to test against
	type FooMsg struct {
		X, Y uint64
		Name string
	}

	msg := FooMsg{1, 2, "foo"}
	// set up the Message.
	iMsg := ecs.NewMessageType[FooMsg, EmptyMsgResult]("FooMsg", ecs.WithMsgEVMSupport[FooMsg, EmptyMsgResult]())
	bz, err := iMsg.ABIEncode(msg)
	assert.NilError(t, err)

	// decode the evm bytes
	fooMsg, err := iMsg.DecodeEVMBytes(bz)
	assert.NilError(t, err)

	// we should be able to cast back to our concrete Go struct.
	f, ok := fooMsg.(FooMsg)
	assert.Equal(t, ok, true)
	assert.DeepEqual(t, f, msg)
}

func TestCannotDecodeEVMBeforeSetEVM(t *testing.T) {
	type foo struct{}
	msg := ecs.NewMessageType[foo, EmptyMsgResult]("foo")
	_, err := msg.DecodeEVMBytes([]byte{})
	assert.ErrorIs(t, err, ecs.ErrEVMTypeNotSet)
}

func TestCannotHaveDuplicateTransactionNames(t *testing.T) {
	type SomeMsg struct {
		X, Y, Z int
	}
	type OtherMsg struct {
		Alpha, Beta string
	}
	engine := testutils.NewTestFixture(t, nil).Engine
	alphaMsg := ecs.NewMessageType[SomeMsg, EmptyMsgResult]("name_match")
	betaMsg := ecs.NewMessageType[OtherMsg, EmptyMsgResult]("name_match")
	assert.ErrorIs(t, engine.RegisterMessages(alphaMsg, betaMsg), msgs.ErrDuplicateMessageName)
}

func TestCanGetTransactionErrorsAndResults(t *testing.T) {
	type MoveMsg struct {
		DeltaX, DeltaY int
	}
	type MoveMsgResult struct {
		EndX, EndY int
	}
	eng := testutils.NewTestFixture(t, nil).Engine

	// Each transaction now needs an input and an output
	moveMsg := ecs.NewMessageType[MoveMsg, MoveMsgResult]("move")
	assert.NilError(t, eng.RegisterMessages(moveMsg))

	wantFirstError := errors.New("this is a transaction error")
	wantSecondError := errors.New("another transaction error")
	wantDeltaX, wantDeltaY := 99, 100

	err := eng.RegisterSystems(
		func(eCtx engine.Context) error {
			// This new In function returns a triplet of information:
			// 1) The transaction input
			// 2) An ID that uniquely identifies this specific transaction
			// 3) The signature
			// This function would replace both "In" and "TxsAndSigsIn"
			txData := moveMsg.In(eCtx)
			assert.Equal(t, 1, len(txData), "expected 1 move transaction")
			tx := txData[0]
			// The input for the transaction is found at tx.Val
			assert.Equal(t, wantDeltaX, tx.Msg.DeltaX)
			assert.Equal(t, wantDeltaY, tx.Msg.DeltaY)

			// AddError will associate an error with the tx.TxHash. Multiple errors can be
			// associated with a transaction.
			moveMsg.AddError(eCtx, tx.Hash, wantFirstError)
			moveMsg.AddError(eCtx, tx.Hash, wantSecondError)

			// SetResult sets the output for the transaction. Only one output can be set
			// for a tx.TxHash (the last assigned result will clobber other results)
			moveMsg.SetResult(eCtx, tx.Hash, MoveMsgResult{42, 42})
			return nil
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())
	_ = moveMsg.AddToQueue(eng, MoveMsg{99, 100})

	// Tick the game so the transaction is processed
	assert.NilError(t, eng.Tick(context.Background()))

	tick := eng.CurrentTick() - 1
	receipts, err := eng.GetTransactionReceiptsForTick(tick)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	r := receipts[0]
	assert.Equal(t, 2, len(r.Errs))
	assert.ErrorIs(t, wantFirstError, r.Errs[0])
	assert.ErrorIs(t, wantSecondError, r.Errs[1])
	got, ok := r.Result.(MoveMsgResult)
	assert.Check(t, ok)
	assert.Equal(t, MoveMsgResult{42, 42}, got)
}

func TestSystemCanFindErrorsFromEarlierSystem(t *testing.T) {
	type MsgIn struct {
		Number int
	}
	type MsgOut struct {
		Number int
	}
	eng := testutils.NewTestFixture(t, nil).Engine
	numTx := ecs.NewMessageType[MsgIn, MsgOut]("number")
	assert.NilError(t, eng.RegisterMessages(numTx))
	wantErr := errors.New("some transaction error")
	systemCalls := 0
	err := eng.RegisterSystems(
		func(eCtx engine.Context) error {
			systemCalls++
			txs := numTx.In(eCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			_, _, ok := numTx.GetReceipt(eCtx, hash)
			assert.Check(t, !ok)
			numTx.AddError(eCtx, hash, wantErr)
			return nil
		},
	)
	assert.NilError(t, err)

	err = eng.RegisterSystems(
		func(eCtx engine.Context) error {
			systemCalls++
			txs := numTx.In(eCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			_, errs, ok := numTx.GetReceipt(eCtx, hash)
			assert.Check(t, ok)
			assert.Equal(t, 1, len(errs))
			assert.ErrorIs(t, wantErr, errs[0])
			return nil
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())

	_ = numTx.AddToQueue(eng, MsgIn{100})

	assert.NilError(t, eng.Tick(context.Background()))
	assert.Equal(t, 2, systemCalls)
}

func TestSystemCanClobberTransactionResult(t *testing.T) {
	type MsgIn struct {
		Number int
	}
	type MsgOut struct {
		Number int
	}
	eng := testutils.NewTestFixture(t, nil).Engine
	numTx := ecs.NewMessageType[MsgIn, MsgOut]("number")
	assert.NilError(t, eng.RegisterMessages(numTx))
	systemCalls := 0

	firstResult := MsgOut{1234}
	secondResult := MsgOut{5678}
	err := eng.RegisterSystems(
		func(eCtx engine.Context) error {
			systemCalls++
			txs := numTx.In(eCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			_, _, ok := numTx.GetReceipt(eCtx, hash)
			assert.Check(t, !ok)
			numTx.SetResult(eCtx, hash, firstResult)
			return nil
		},
	)
	assert.NilError(t, err)

	err = eng.RegisterSystems(
		func(eCtx engine.Context) error {
			systemCalls++
			txs := numTx.In(eCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			out, errs, ok := numTx.GetReceipt(eCtx, hash)
			assert.Check(t, ok)
			assert.Equal(t, 0, len(errs))
			assert.Equal(t, MsgOut{1234}, out)
			numTx.SetResult(eCtx, hash, secondResult)
			return nil
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())

	_ = numTx.AddToQueue(eng, MsgIn{100})

	assert.NilError(t, eng.Tick(context.Background()))

	prevTick := eng.CurrentTick() - 1
	receipts, err := eng.GetTransactionReceiptsForTick(prevTick)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	r := receipts[0]
	assert.Equal(t, 0, len(r.Errs))
	gotResult, ok := r.Result.(MsgOut)
	assert.Check(t, ok)
	assert.Equal(t, secondResult, gotResult)
}

func TestCopyTransactions(t *testing.T) {
	type FooMsg struct {
		X int
	}
	txq := txpool.NewTxQueue()
	txq.AddTransaction(1, FooMsg{X: 3}, &sign.Transaction{PersonaTag: "foo"})
	txq.AddTransaction(2, FooMsg{X: 4}, &sign.Transaction{PersonaTag: "bar"})

	copyTxq := txq.CopyTransactions()
	assert.Equal(t, copyTxq.GetAmountOfTxs(), 2)
	assert.Equal(t, txq.GetAmountOfTxs(), 0)
}

func TestNewTransactionPanicsIfNoName(t *testing.T) {
	type Foo struct{}
	require.Panics(
		t, func() {
			ecs.NewMessageType[Foo, Foo]("")
		},
	)
}
