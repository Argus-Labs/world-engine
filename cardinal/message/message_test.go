package message_test

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/sign"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal"
)

type Health struct {
	Value int
}

func (Health) Name() string { return "health" }

type AddHealthToEntityTx struct {
	TargetID types.EntityID
	Amount   int
}

type AddHealthToEntityResult struct{}

var addHealthToEntity = message.NewMessageType[AddHealthToEntityTx, AddHealthToEntityResult]("add_health")

func TestTransactionExample(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world, doTick := tf.World, tf.DoTick
	assert.NilError(t, cardinal.RegisterComponent[Health](world))
	assert.NilError(t, cardinal.RegisterMessages(world, addHealthToEntity))
	err := cardinal.RegisterSystems(world, func(wCtx engine.Context) error {
		// test "In" method
		for _, tx := range addHealthToEntity.In(wCtx) {
			targetID := tx.Msg.TargetID
			err := cardinal.UpdateComponent[Health](wCtx, targetID, func(h *Health) *Health {
				h.Value = tx.Msg.Amount
				return h
			})
			assert.Check(t, err == nil)
		}
		// test same as above but with forEach
		addHealthToEntity.Each(wCtx,
			func(tx message.TxData[AddHealthToEntityTx]) (AddHealthToEntityResult, error) {
				targetID := tx.Msg.TargetID
				err := cardinal.UpdateComponent[Health](wCtx, targetID,
					func(h *Health) *Health {
						h.Value = tx.Msg.Amount
						return h
					})
				assert.Check(t, err == nil)
				return AddHealthToEntityResult{}, errors.New("fake tx error")
			})

		return nil
	})
	assert.NilError(t, err)

	testWorldCtx := cardinal.NewWorldContext(world)
	doTick()
	ids, err := cardinal.CreateMany(testWorldCtx, 10, Health{})
	assert.NilError(t, err)

	// Queue up the transaction.
	idToModify := ids[3]
	amountToModify := 20
	payload := testutils.UniqueSignature()
	testutils.AddTransactionToWorldByAnyTransaction(
		world, addHealthToEntity,
		AddHealthToEntityTx{idToModify, amountToModify}, payload,
	)

	// The health change should be applied during this tick
	doTick()

	// Make sure the target entity had its health updated.
	for _, id := range ids {
		var health *Health
		health, err = cardinal.GetComponent[Health](testWorldCtx, id)
		assert.NilError(t, err)
		if id == idToModify {
			assert.Equal(t, amountToModify, health.Value)
		} else {
			assert.Equal(t, 0, health.Value)
		}
	}
	// Make sure transaction errors are recorded in the receipt
	receipts, err := testWorldCtx.GetTransactionReceiptsForTick(testWorldCtx.CurrentTick() - 1)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	assert.Equal(t, 1, len(receipts[0].Errs))
}

func TestForEachTransaction(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	type SomeMsgRequest struct {
		GenerateError bool
	}
	type SomeMsgResponse struct {
		Successful bool
	}

	someMsg := message.NewMessageType[SomeMsgRequest, SomeMsgResponse]("some_msg")
	assert.NilError(t, cardinal.RegisterMessages(world, someMsg))

	err := cardinal.RegisterSystems(world, func(wCtx engine.Context) error {
		someMsg.Each(wCtx, func(t message.TxData[SomeMsgRequest]) (result SomeMsgResponse, err error) {
			if t.Msg.GenerateError {
				return result, errors.New("some error")
			}
			return SomeMsgResponse{
				Successful: true,
			}, nil
		})
		return nil
	})
	assert.NilError(t, err)
	tf.StartWorld()

	// Add 10 transactions to the tx queue and keep track of the hashes that we just cardinal.Created
	knownTxHashes := map[types.TxHash]SomeMsgRequest{}
	for i := 0; i < 10; i++ {
		req := SomeMsgRequest{GenerateError: i%2 == 0}
		txHash := tf.AddTransaction(someMsg.ID(), req, testutils.UniqueSignature())
		knownTxHashes[txHash] = req
	}

	// Perform a engine tick
	assert.NilError(t, world.Tick(context.Background()))

	// Verify the receipts for the previous tick are what we expect
	receipts, err := world.GetTransactionReceiptsForTick(world.CurrentTick() - 1)
	assert.NilError(t, err)
	assert.Equal(t, len(knownTxHashes), len(receipts))
	for _, receipt := range receipts {
		request, ok := knownTxHashes[receipt.TxHash]
		assert.Check(t, ok)
		if request.GenerateError {
			assert.Check(t, len(receipt.Errs) > 0)
		} else {
			assert.Equal(t, 0, len(receipt.Errs))
			assert.Equal(t, receipt.Result.(SomeMsgResponse), SomeMsgResponse{Successful: true})
		}
	}
}

type ScoreComponent struct {
	Score int
}

func (ScoreComponent) Name() string {
	return "score"
}

type ModifyScoreMsg struct {
	PlayerID types.EntityID
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

		message.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_score2")
	}()
	message.NewMessageType[string, string]("modify_score1")
}

func TestCanQueueTransactions(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	// cardinal.Create an entity with a score component
	assert.NilError(t, cardinal.RegisterComponent[ScoreComponent](world))
	modifyScoreMsg := message.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_score")
	assert.NilError(t, cardinal.RegisterMessages(world, modifyScoreMsg))

	wCtx := cardinal.NewWorldContext(world)

	// Set up a system that allows for the modification of a player's score
	err := cardinal.RegisterSystems(world,
		func(wCtx engine.Context) error {
			modifyScore := modifyScoreMsg.In(wCtx)
			for _, txData := range modifyScore {
				ms := txData.Msg
				err := cardinal.UpdateComponent[ScoreComponent](
					wCtx, ms.PlayerID, func(s *ScoreComponent) *ScoreComponent {
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
	tf.StartWorld()
	id, err := cardinal.Create(wCtx, ScoreComponent{})
	assert.NilError(t, err)

	tf.AddTransaction(modifyScoreMsg.ID(), &ModifyScoreMsg{id, 100})

	assert.NilError(t, cardinal.SetComponent[ScoreComponent](wCtx, id, &ScoreComponent{}))

	// Verify the score is 0
	s, err := cardinal.GetComponent[ScoreComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 0, s.Score)

	// Process a game tick
	assert.NilError(t, world.Tick(context.Background()))

	// Verify the score was updated
	s, err = cardinal.GetComponent[ScoreComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 100, s.Score)

	// Tick again, but no new modifyScoreMsg was added to the queue
	assert.NilError(t, world.Tick(context.Background()))

	// Verify the score hasn't changed
	s, err = cardinal.GetComponent[ScoreComponent](wCtx, id)
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[CounterComponent](world))

	wCtx := cardinal.NewWorldContext(world)

	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			search := cardinal.NewSearch(wCtx, filter.Exact(CounterComponent{}))
			id := search.MustFirst()
			return cardinal.UpdateComponent[CounterComponent](
				wCtx, id, func(c *CounterComponent) *CounterComponent {
					c.Count++
					return c
				},
			)
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	id, err := cardinal.Create(wCtx, CounterComponent{})
	assert.NilError(t, err)

	for i := 0; i < 10; i++ {
		assert.NilError(t, world.Tick(context.Background()))
	}

	c, err := cardinal.GetComponent[CounterComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 10, c.Count)
}

func TestTransactionAreAppliedToSomeEntities(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ScoreComponent](world))

	modifyScoreMsg := message.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_score")
	assert.NilError(t, cardinal.RegisterMessages(world, modifyScoreMsg))

	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			modifyScores := modifyScoreMsg.In(wCtx)
			for _, msData := range modifyScores {
				ms := msData.Msg
				err := cardinal.UpdateComponent[ScoreComponent](
					wCtx, ms.PlayerID, func(s *ScoreComponent) *ScoreComponent {
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
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	ids, err := cardinal.CreateMany(wCtx, 100, ScoreComponent{})
	assert.NilError(t, err)
	// Entities at index 5, 10 and 50 will be updated with some values
	tf.AddTransaction(
		modifyScoreMsg.ID(), &ModifyScoreMsg{
			PlayerID: ids[5],
			Amount:   105,
		},
	)
	tf.AddTransaction(
		modifyScoreMsg.ID(), &ModifyScoreMsg{
			PlayerID: ids[10],
			Amount:   110,
		},
	)
	tf.AddTransaction(
		modifyScoreMsg.ID(), &ModifyScoreMsg{
			PlayerID: ids[50],
			Amount:   150,
		},
	)

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
		s, err := cardinal.GetComponent[ScoreComponent](wCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, wantScore, s.Score)
	}
}

// TestAddToQueueDuringTickDoesNotTimeout verifies that we can add a transaction to the transaction
// queue during a game tick, and the call does not block.
func TestAddToQueueDuringTickDoesNotTimeout(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	modScore := message.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_Score")
	assert.NilError(t, cardinal.RegisterMessages(world, modScore))

	inSystemCh := make(chan struct{})
	// This system will block forever. This will give us a never-ending game tick that we can use
	// to verify that the addition of more transactions doesn't block.
	err := cardinal.RegisterSystems(
		world,
		func(engine.Context) error {
			<-inSystemCh
			select {}
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()

	tf.AddTransaction(modScore.ID(), &ModifyScoreMsg{})

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
		tf.AddTransaction(modScore.ID(), &ModifyScoreMsg{})
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	modScoreMsg := message.NewMessageType[*ModifyScoreMsg, *EmptyMsgResult]("modify_score")
	assert.NilError(t, cardinal.RegisterMessages(world, modScoreMsg))
	tickStart := tf.StartTickCh
	tickDone := tf.DoneTickCh

	modScoreCountCh := make(chan int)

	// Create two system that report how many instances of the ModifyScoreMsg exist in the
	// transaction queue. These counts should be the same for each tick. modScoreCountCh is an unbuffered channel
	// so these systems will block while writing to modScoreCountCh. This allows the test to ensure that we can run
	// commands mid-tick.
	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			modScores := modScoreMsg.In(wCtx)
			modScoreCountCh <- len(modScores)
			return nil
		},
	)
	assert.NilError(t, err)

	err = cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			modScores := modScoreMsg.In(wCtx)
			modScoreCountCh <- len(modScores)
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()

	tf.AddTransaction(modScoreMsg.ID(), &ModifyScoreMsg{})

	// Start the game tick. The tick will block while waiting to write to modScoreCountCh
	tickStart <- time.Now()

	// In the first system, we should see 1 modify score transaction
	count := <-modScoreCountCh
	assert.Equal(t, 1, count)

	// Add two transactions mid-tick.
	tf.AddTransaction(modScoreMsg.ID(), &ModifyScoreMsg{})
	tf.AddTransaction(modScoreMsg.ID(), &ModifyScoreMsg{})

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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	type NewOwner struct {
		Name string
	}

	alpha := message.NewMessageType[NewOwner, EmptyMsgResult]("alpha_msg")
	beta := message.NewMessageType[NewOwner, EmptyMsgResult]("beta_msg")
	assert.NilError(t, cardinal.RegisterMessages(world, alpha, beta))

	tf.AddTransaction(alpha.ID(), NewOwner{"alpha"})
	tf.AddTransaction(beta.ID(), NewOwner{"beta"})

	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			newNames := alpha.In(wCtx)
			assert.Check(t, len(newNames) == 1, "expected 1 transaction, not %d", len(newNames))
			assert.Check(t, newNames[0].Msg.Name == "alpha")

			newNames = beta.In(wCtx)
			assert.Check(t, len(newNames) == 1, "expected 1 transaction, not %d", len(newNames))
			assert.Check(t, newNames[0].Msg.Name == "beta")
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()

	assert.NilError(t, world.Tick(context.Background()))
}

func TestCannotRegisterDuplicateTransaction(t *testing.T) {
	msg := message.NewMessageType[ModifyScoreMsg, EmptyMsgResult]("modify_score")
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.Check(t, nil != cardinal.RegisterMessages(world, msg, msg))
}

func TestCannotCallRegisterTransactionsMultipleTimes(t *testing.T) {
	msg := message.NewMessageType[ModifyScoreMsg, EmptyMsgResult]("modify_score")
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterMessages(world, msg))
	assert.Check(t, nil != cardinal.RegisterMessages(world, msg))
}

func TestCanEncodeDecodeEVMTransactions(t *testing.T) {
	// the msg we are going to test against
	type FooMsg struct {
		X, Y uint64
		Name string
	}

	msg := FooMsg{1, 2, "foo"}
	// set up the Message.
	iMsg := message.NewMessageType[FooMsg, EmptyMsgResult]("FooMsg",
		message.WithMsgEVMSupport[FooMsg, EmptyMsgResult]())
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
	msg := message.NewMessageType[foo, EmptyMsgResult]("foo")
	_, err := msg.DecodeEVMBytes([]byte{})
	assert.ErrorIs(t, err, message.ErrEVMTypeNotSet)
}

func TestCannotHaveDuplicateTransactionNames(t *testing.T) {
	type SomeMsg struct {
		X, Y, Z int
	}
	type OtherMsg struct {
		Alpha, Beta string
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	alphaMsg := message.NewMessageType[SomeMsg, EmptyMsgResult]("name_match")
	betaMsg := message.NewMessageType[OtherMsg, EmptyMsgResult]("name_match")
	assert.IsError(t, cardinal.RegisterMessages(world, alphaMsg, betaMsg))
}

func TestCanGetTransactionErrorsAndResults(t *testing.T) {
	type MoveMsg struct {
		DeltaX, DeltaY int
	}
	type MoveMsgResult struct {
		EndX, EndY int
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	// Each transaction now needs an input and an output
	moveMsg := message.NewMessageType[MoveMsg, MoveMsgResult]("move")
	assert.NilError(t, cardinal.RegisterMessages(world, moveMsg))

	wantFirstError := errors.New("this is a transaction error")
	wantSecondError := errors.New("another transaction error")
	wantDeltaX, wantDeltaY := 99, 100

	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			// This new In function returns a triplet of information:
			// 1) The transaction input
			// 2) An EntityID that uniquely identifies this specific transaction
			// 3) The signature
			// This function would replace both "In" and "TxsAndSigsIn"
			txData := moveMsg.In(wCtx)
			assert.Equal(t, 1, len(txData), "expected 1 move transaction")
			tx := txData[0]
			// The input for the transaction is found at tx.Val
			assert.Equal(t, wantDeltaX, tx.Msg.DeltaX)
			assert.Equal(t, wantDeltaY, tx.Msg.DeltaY)

			// AddError will associate an error with the tx.TxHash. Multiple errors can be
			// associated with a transaction.
			moveMsg.AddError(wCtx, tx.Hash, wantFirstError)
			moveMsg.AddError(wCtx, tx.Hash, wantSecondError)

			// SetResult sets the output for the transaction. Only one output can be set
			// for a tx.TxHash (the last assigned result will clobber other results)
			moveMsg.SetResult(wCtx, tx.Hash, MoveMsgResult{42, 42})
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	_ = tf.AddTransaction(moveMsg.ID(), MoveMsg{99, 100})

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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	numTx := message.NewMessageType[MsgIn, MsgOut]("number")
	assert.NilError(t, cardinal.RegisterMessages(world, numTx))
	wantErr := errors.New("some transaction error")
	systemCalls := 0
	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			systemCalls++
			txs := numTx.In(wCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			_, _, ok := numTx.GetReceipt(wCtx, hash)
			assert.Check(t, !ok)
			numTx.AddError(wCtx, hash, wantErr)
			return nil
		},
	)
	assert.NilError(t, err)

	err = cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			systemCalls++
			txs := numTx.In(wCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			_, errs, ok := numTx.GetReceipt(wCtx, hash)
			assert.Check(t, ok)
			assert.Equal(t, 1, len(errs))
			assert.ErrorIs(t, wantErr, errs[0])
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()

	_ = tf.AddTransaction(numTx.ID(), MsgIn{100})

	assert.NilError(t, world.Tick(context.Background()))
	assert.Equal(t, 2, systemCalls)
}

func TestSystemCanClobberTransactionResult(t *testing.T) {
	type MsgIn struct {
		Number int
	}
	type MsgOut struct {
		Number int
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	numTx := message.NewMessageType[MsgIn, MsgOut]("number")
	assert.NilError(t, cardinal.RegisterMessages(world, numTx))
	systemCalls := 0

	firstResult := MsgOut{1234}
	secondResult := MsgOut{5678}
	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			systemCalls++
			txs := numTx.In(wCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			_, _, ok := numTx.GetReceipt(wCtx, hash)
			assert.Check(t, !ok)
			numTx.SetResult(wCtx, hash, firstResult)
			return nil
		},
	)
	assert.NilError(t, err)

	err = cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			systemCalls++
			txs := numTx.In(wCtx)
			assert.Equal(t, 1, len(txs))
			hash := txs[0].Hash
			out, errs, ok := numTx.GetReceipt(wCtx, hash)
			assert.Check(t, ok)
			assert.Equal(t, 0, len(errs))
			assert.Equal(t, MsgOut{1234}, out)
			numTx.SetResult(wCtx, hash, secondResult)
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()

	_ = tf.AddTransaction(numTx.ID(), MsgIn{100})

	assert.NilError(t, world.Tick(context.Background()))

	prevTick := world.CurrentTick() - 1
	receipts, err := world.GetTransactionReceiptsForTick(prevTick)
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
			message.NewMessageType[Foo, Foo]("")
		},
	)
}
