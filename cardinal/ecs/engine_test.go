package ecs_test

import (
	"context"
	"errors"
	"github.com/golang/mock/gomock"
	"pkg.world.dev/world-engine/cardinal/router/iterator"
	"pkg.world.dev/world-engine/cardinal/router/mocks"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/sign"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
)

func TestCanWaitForNextTick(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine
	startTickCh := make(chan time.Time)
	doneTickCh := make(chan uint64)
	assert.NilError(t, engine.LoadGameState())
	engine.StartGameLoop(context.Background(), startTickCh, doneTickCh)

	// Make sure the game can tick
	startTickCh <- time.Now()
	<-doneTickCh

	waitForNextTickDone := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			success := engine.WaitForNextTick()
			assert.Check(t, success)
		}
		close(waitForNextTickDone)
	}()

	for {
		select {
		case startTickCh <- time.Now():
			<-doneTickCh
		case <-waitForNextTickDone:
			// The above goroutine successfully waited multiple times
			return
		}
	}
}

func TestWaitForNextTickReturnsFalseWhenEngineIsShutDown(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine
	startTickCh := make(chan time.Time)
	doneTickCh := make(chan uint64)
	assert.NilError(t, engine.LoadGameState())
	engine.StartGameLoop(context.Background(), startTickCh, doneTickCh)

	// Make sure the game can tick
	startTickCh <- time.Now()
	<-doneTickCh

	waitForNextTickDone := make(chan struct{})
	go func() {
		// continually spin here waiting for next tick. One of these must fail before
		// the test times out for this test to pass
		for engine.WaitForNextTick() {
		}
		close(waitForNextTickDone)
	}()

	// Shutdown the engine at some point in the near future
	time.AfterFunc(
		100*time.Millisecond, func() {
			assert.NilError(t, engine.Shutdown())
		},
	)
	// testTimeout will cause the test to fail if we have to wait too long for a WaitForNextTick failure
	testTimeout := time.After(5 * time.Second)
	for {
		select {
		case startTickCh <- time.Now():
			time.Sleep(10 * time.Millisecond)
			<-doneTickCh
		case <-testTimeout:
			assert.Check(t, false, "test timeout")
			return
		case <-waitForNextTickDone:
			// WaitForNextTick failed, meaning this test was successful
			return
		}
	}
}

func TestCannotWaitForNextTickAfterEngineIsShutDown(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine
	startTickCh := make(chan time.Time)
	doneTickCh := make(chan uint64)
	assert.NilError(t, engine.LoadGameState())
	engine.StartGameLoop(context.Background(), startTickCh, doneTickCh)

	// Make sure the game can tick
	startTickCh <- time.Now()
	<-doneTickCh

	assert.NilError(t, engine.Shutdown())

	for i := 0; i < 10; i++ {
		// After a engine is shut down, WaitForNextTick should never block and always fail
		assert.Check(t, !engine.WaitForNextTick())
	}
}

func TestEVMTxConsume(t *testing.T) {
	ctx := context.Background()
	type FooIn struct {
		X uint32
	}
	type FooOut struct {
		Y string
	}
	e := testutils.NewTestFixture(t, nil).Engine
	fooTx := ecs.NewMessageType[FooIn, FooOut]("foo", ecs.WithMsgEVMSupport[FooIn, FooOut]())
	assert.NilError(t, e.RegisterMessages(fooTx))
	var returnVal FooOut
	var returnErr error
	e.RegisterSystem(
		func(eCtx ecs.EngineContext) error {
			fooTx.Each(
				eCtx, func(t ecs.TxData[FooIn]) (FooOut, error) {
					return returnVal, returnErr
				},
			)
			return nil
		},
	)
	assert.NilError(t, e.LoadGameState())

	// add tx to queue
	evmTxHash := "0xFooBar"
	e.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)

	// let's check against a system that returns a result and no error
	returnVal = FooOut{Y: "hi"}
	returnErr = nil
	assert.NilError(t, e.Tick(ctx))
	evmTxReceipt, ok := e.GetEVMMsgResult(evmTxHash)
	assert.Equal(t, ok, true)
	assert.Check(t, len(evmTxReceipt.ABIResult) > 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 0)
	// shouldn't be able to consume it again.
	_, ok = e.GetEVMMsgResult(evmTxHash)
	assert.Equal(t, ok, false)

	// lets check against a system that returns an error
	returnVal = FooOut{}
	returnErr = errors.New("omg error")
	e.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)
	assert.NilError(t, e.Tick(ctx))
	evmTxReceipt, ok = e.GetEVMMsgResult(evmTxHash)

	assert.Equal(t, ok, true)
	assert.Equal(t, len(evmTxReceipt.ABIResult), 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 1)
	// shouldn't be able to consume it again.
	_, ok = e.GetEVMMsgResult(evmTxHash)
	assert.Equal(t, ok, false)
}

func TestAddSystems(t *testing.T) {
	count := 0
	sys := func(ecs.EngineContext) error {
		count++
		return nil
	}

	engine := testutils.NewTestFixture(t, nil).Engine
	engine.RegisterSystems(sys, sys, sys)
	err := engine.LoadGameState()
	assert.NilError(t, err)

	err = engine.Tick(context.Background())
	assert.NilError(t, err)

	assert.Equal(t, count, 3)
}

func TestSystemExecutionOrder(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine
	order := make([]int, 0, 3)
	engine.RegisterSystems(
		func(ecs.EngineContext) error {
			order = append(order, 1)
			return nil
		}, func(ecs.EngineContext) error {
			order = append(order, 2)
			return nil
		}, func(ecs.EngineContext) error {
			order = append(order, 3)
			return nil
		},
	)
	err := engine.LoadGameState()
	assert.NilError(t, err)
	assert.NilError(t, engine.Tick(context.Background()))
	expectedOrder := []int{1, 2, 3}
	for i, elem := range order {
		assert.Equal(t, elem, expectedOrder[i])
	}
}

func TestSetNamespace(t *testing.T) {
	namespace := "test"
	t.Setenv("CARDINAL_NAMESPACE", namespace)
	e := testutils.NewTestFixture(t, nil).Engine
	assert.Equal(t, e.Namespace().String(), namespace)
}

func TestWithoutRegistration(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine
	eCtx := ecs.NewEngineContext(engine)
	id, err := ecs.Create(eCtx, EnergyComponent{}, OwnableComponent{})
	assert.Assert(t, err != nil)

	err = ecs.UpdateComponent[EnergyComponent](
		eCtx, id, func(component *EnergyComponent) *EnergyComponent {
			component.Amt += 50
			return component
		},
	)
	assert.Assert(t, err != nil)

	err = ecs.SetComponent[EnergyComponent](
		eCtx, id, &EnergyComponent{
			Amt: 0,
			Cap: 0,
		},
	)

	assert.Assert(t, err != nil)

	assert.NilError(t, ecs.RegisterComponent[EnergyComponent](engine))
	assert.NilError(t, ecs.RegisterComponent[OwnableComponent](engine))
	assert.NilError(t, engine.LoadGameState())

	id, err = ecs.Create(eCtx, EnergyComponent{}, OwnableComponent{})
	assert.NilError(t, err)
	err = ecs.UpdateComponent[EnergyComponent](
		eCtx, id, func(component *EnergyComponent) *EnergyComponent {
			component.Amt += 50
			return component
		},
	)
	assert.NilError(t, err)
	err = ecs.SetComponent[EnergyComponent](
		eCtx, id, &EnergyComponent{
			Amt: 0,
			Cap: 0,
		},
	)
	assert.NilError(t, err)
}

func TestTransactionsSentToRouterAfterTick(t *testing.T) {
	ctrl := gomock.NewController(t)
	rtr := mocks.NewMockRouter(ctrl)
	engine := testutils.NewTestFixture(t, nil).Engine
	engine.SetRouter(rtr)
	type fooMsg struct {
		Bar string
	}

	type fooMsgRes struct{}
	fooMessage := ecs.NewMessageType[fooMsg, fooMsgRes]("foo", ecs.WithMsgEVMSupport[fooMsg, fooMsgRes]())
	err := engine.RegisterMessages(fooMessage)
	assert.NilError(t, err)

	err = engine.LoadGameState()
	assert.NilError(t, err)

	evmTxHash := "0x12345"
	msg := fooMsg{Bar: "hello"}
	tx := &sign.Transaction{PersonaTag: "ty"}
	_, txHash := engine.AddEVMTransaction(fooMessage.ID(), msg, tx, evmTxHash)

	rtr.
		EXPECT().
		SubmitTxBlob(
			gomock.Any(),
			txpool.TxMap{fooMessage.ID(): {{
				MsgID:           fooMessage.ID(),
				Msg:             msg,
				TxHash:          txHash,
				Tx:              tx,
				EVMSourceTxHash: evmTxHash,
			}}},
			engine.CurrentTick(),
			gomock.Any(),
		).
		Return(nil).
		Times(1)
	err = engine.Tick(context.Background())
	assert.NilError(t, err)
}

type mockIterator struct {
	batches []mockBatches
}

type mockBatches struct {
	batch     []*iterator.TxBatch
	tick      uint64
	timestamp uint64
}

func (m *mockIterator) Each(fn func(batches []*iterator.TxBatch, tick, timestamp uint64) error) error {
	for _, b := range m.batches {
		if err := fn(b.batch, b.tick, b.timestamp); err != nil {
			return err
		}
	}
	return nil
}

func TestRecoverFromChain(t *testing.T) {
	ctrl := gomock.NewController(t)
	rtr := mocks.NewMockRouter(ctrl)
	engine := testutils.NewTestFixture(t, nil).Engine
	engine.SetRouter(rtr)

	type fooMsg struct{ I int }
	type fooMsgRes struct{}
	fooMsgName := "foo"
	fooMessage := ecs.NewMessageType[fooMsg, fooMsgRes](fooMsgName)
	err := engine.RegisterMessages(fooMessage)
	assert.NilError(t, err)

	fooMessages := 0
	engine.RegisterSystem(func(engineContext ecs.EngineContext) error {
		fooMessage.Each(engineContext, func(t ecs.TxData[fooMsg]) (fooMsgRes, error) {
			fooMessages++
			return fooMsgRes{}, nil
		})
		return nil
	})
	err = engine.LoadGameState()
	assert.NilError(t, err)

	it := mockIterator{batches: []mockBatches{
		{
			batch: []*iterator.TxBatch{
				{
					Tx:       &sign.Transaction{PersonaTag: "ty"},
					MsgID:    fooMessage.ID(),
					MsgValue: fooMsg{I: 40},
				},
				{
					Tx:       &sign.Transaction{PersonaTag: "ty"},
					MsgID:    fooMessage.ID(),
					MsgValue: fooMsg{I: 2240},
				},
			},
			tick:      3,
			timestamp: 420,
		},
		{
			batch: []*iterator.TxBatch{
				{
					Tx:       &sign.Transaction{PersonaTag: "ty"},
					MsgID:    fooMessage.ID(),
					MsgValue: fooMsg{I: 420},
				},
				{
					Tx:       &sign.Transaction{PersonaTag: "ty"},
					MsgID:    fooMessage.ID(),
					MsgValue: fooMsg{I: 33},
				},
			},
			tick:      10,
			timestamp: 490,
		},
	}}
	rtr.EXPECT().TransactionIterator().Return(&it).Times(1)
	err = engine.RecoverFromChain(context.Background())
	assert.Equal(t, fooMessages, 4)
	assert.Equal(t, engine.CurrentTick(), uint64(11))
}

func TestCantRecoverWithoutRouter(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine
	err := engine.RecoverFromChain(context.Background())
	assert.ErrorContains(t, err, "cannot recover state without router")
}

func TestCantRecoverWhenTickIsGreaterThanZero(t *testing.T) {
	ctrl := gomock.NewController(t)
	rtr := mocks.NewMockRouter(ctrl)
	engine := testutils.NewTestFixture(t, nil).Engine
	engine.SetRouter(rtr)
	err := engine.LoadGameState()
	assert.NilError(t, err)
	err = engine.Tick(context.Background())
	assert.NilError(t, err)

	err = engine.RecoverFromChain(context.Background())
	assert.ErrorContains(t, err, "cannot recover world with existing state")
}

func TestRecoveredOutOfOrderTick(t *testing.T) {
	ctrl := gomock.NewController(t)
	rtr := mocks.NewMockRouter(ctrl)
	engine := testutils.NewTestFixture(t, nil).Engine
	engine.SetRouter(rtr)
	err := engine.LoadGameState()
	assert.NilError(t, err)

	it := mockIterator{batches: []mockBatches{
		{
			batch: []*iterator.TxBatch{
				{
					Tx:       &sign.Transaction{PersonaTag: "ty"},
					MsgID:    message.TypeID(30),
					MsgValue: "hi",
				},
			},
			tick:      3,
			timestamp: 420,
		},
		{
			batch: []*iterator.TxBatch{
				{
					Tx:       &sign.Transaction{PersonaTag: "ty"},
					MsgID:    message.TypeID(30),
					MsgValue: "hi",
				},
			},
			tick:      2,
			timestamp: 420,
		},
	}}
	rtr.EXPECT().TransactionIterator().Return(&it).Times(1)
	err = engine.RecoverFromChain(context.Background())
	assert.ErrorContains(t, err, "got tick for 2 but Cardinal was at 4")
}
