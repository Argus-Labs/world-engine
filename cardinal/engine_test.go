package cardinal_test

import (
	"context"
	"errors"
	"github.com/golang/mock/gomock"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/router/mocks"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/sign"

	"pkg.world.dev/world-engine/assert"
)

func TestCanWaitForNextTick(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	startTickCh := tf.StartTickCh
	doneTickCh := tf.DoneTickCh

	// Make sure the game can tick
	tf.StartWorld()
	tf.DoTick()

	waitForNextTickDone := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			success := world.WaitForNextTick()
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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	startTickCh := tf.StartTickCh
	doneTickCh := tf.DoneTickCh

	// Make sure the game can tick
	tf.StartWorld()
	tf.DoTick()

	waitForNextTickDone := make(chan struct{})
	go func() {
		// continually spin here waiting for next tick. One of these must fail before
		// the test times out for this test to pass
		for world.WaitForNextTick() {
		}
		close(waitForNextTickDone)
	}()

	// Shutdown the engine at some point in the near future
	time.AfterFunc(
		100*time.Millisecond, func() {
			assert.NilError(t, world.Shutdown())
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
	ctx := context.Background()
	type FooIn struct {
		X uint32
	}
	type FooOut struct {
		Y string
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	fooTx := message.NewMessageType[FooIn, FooOut]("foo", message.WithMsgEVMSupport[FooIn, FooOut]())
	assert.NilError(t, cardinal.RegisterMessages(world, fooTx))
	var returnVal FooOut
	var returnErr error
	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			fooTx.Each(
				wCtx, func(t message.TxData[FooIn]) (FooOut, error) {
					return returnVal, returnErr
				},
			)
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()

	// add tx to pool
	evmTxHash := "0xFooBar"
	world.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)

	tf.StartWorld()

	// let's check against a system that returns a result and no error
	returnVal = FooOut{Y: "hi"}
	returnErr = nil
	assert.NilError(t, world.Tick(ctx))
	evmTxReceipt, ok := world.GetEVMMsgReceipt(evmTxHash)
	assert.Equal(t, ok, true)
	assert.Check(t, len(evmTxReceipt.ABIResult) > 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 0)
	// shouldn't be able to consume it again.
	_, ok = world.GetEVMMsgReceipt(evmTxHash)
	assert.Equal(t, ok, false)

	// lets check against a system that returns an error
	returnVal = FooOut{}
	returnErr = errors.New("omg error")
	world.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)
	assert.NilError(t, world.Tick(ctx))
	evmTxReceipt, ok = world.GetEVMMsgReceipt(evmTxHash)

	assert.Equal(t, ok, true)
	assert.Equal(t, len(evmTxReceipt.ABIResult), 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 1)
	// shouldn't be able to consume it again.
	_, ok = world.GetEVMMsgReceipt(evmTxHash)
	assert.Equal(t, ok, false)
}

func TestEVMTxConsume(t *testing.T) {
	ctx := context.Background()
	type FooIn struct {
		X uint32
	}
	type FooOut struct {
		Y string
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	fooTx := message.NewMessageType[FooIn, FooOut]("foo", message.WithMsgEVMSupport[FooIn, FooOut]())
	assert.NilError(t, cardinal.RegisterMessages(world, fooTx))
	var returnVal FooOut
	var returnErr error
	err := cardinal.RegisterSystems(world,
		func(eCtx cardinal.WorldContext) error {
			fooTx.Each(
				eCtx, func(t message.TxData[FooIn]) (FooOut, error) {
					return returnVal, returnErr
				},
			)
			return nil
		},
	)
	assert.NilError(t, err)

	tf.StartWorld()

	// add tx to pool
	evmTxHash := "0xFooBar"
	world.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)

	// let's check against a system that returns a result and no error
	returnVal = FooOut{Y: "hi"}
	returnErr = nil
	assert.NilError(t, world.Tick(ctx))
	evmTxReceipt, ok := world.GetEVMMsgReceipt(evmTxHash)
	assert.Equal(t, ok, true)
	assert.Check(t, len(evmTxReceipt.ABIResult) > 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 0)
	// shouldn't be able to consume it again.
	_, ok = world.GetEVMMsgReceipt(evmTxHash)
	assert.Equal(t, ok, false)

	// lets check against a system that returns an error
	returnVal = FooOut{}
	returnErr = errors.New("omg error")
	world.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)
	assert.NilError(t, world.Tick(ctx))
	evmTxReceipt, ok = world.GetEVMMsgReceipt(evmTxHash)

	assert.Equal(t, ok, true)
	assert.Equal(t, len(evmTxReceipt.ABIResult), 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 1)
	// shouldn't be able to consume it again.
	_, ok = world.GetEVMMsgReceipt(evmTxHash)
	assert.Equal(t, ok, false)
}

func TestAddSystems(t *testing.T) {
	count := 0
	sys1 := func(engine.Context) error {
		count++
		return nil
	}
	sys2 := func(engine.Context) error {
		count++
		return nil
	}
	sys3 := func(engine.Context) error {
		count++
		return nil
	}

	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	err := cardinal.RegisterSystems(world, sys1, sys2, sys3)
	assert.NilError(t, err)

	tf.StartWorld()
	assert.NilError(t, err)

	err = world.Tick(context.Background())
	assert.NilError(t, err)

	assert.Equal(t, count, 3)
}

func TestSystemExecutionOrder(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	order := make([]int, 0, 3)
	err := cardinal.RegisterSystems(
		world,
		func(engine.Context) error {
			order = append(order, 1)
			return nil
		}, func(engine.Context) error {
			order = append(order, 2)
			return nil
		}, func(engine.Context) error {
			order = append(order, 3)
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	assert.NilError(t, err)
	assert.NilError(t, world.Tick(context.Background()))
	expectedOrder := []int{1, 2, 3}
	for i, elem := range order {
		assert.Equal(t, elem, expectedOrder[i])
	}
}

func TestSetNamespace(t *testing.T) {
	namespace := "test"
	t.Setenv("CARDINAL_NAMESPACE", namespace)
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.Equal(t, world.Namespace().String(), namespace)
}

func TestWithoutRegistration(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	wCtx := cardinal.NewWorldContext(world)

	assert.Panics(t, func() { _, _ = cardinal.Create(wCtx, EnergyComponent{}, OwnableComponent{}) })
	assert.Panics(t, func() {
		_ = cardinal.UpdateComponent[EnergyComponent](
			wCtx, 0, func(component *EnergyComponent) *EnergyComponent {
				component.Amt += 50
				return component
			},
		)
	})
	assert.Panics(t, func() {
		_ = cardinal.SetComponent[EnergyComponent](
			wCtx, 0, &EnergyComponent{
				Amt: 0,
				Cap: 0,
			},
		)
	})

	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world))
	assert.NilError(t, cardinal.RegisterComponent[OwnableComponent](world))
	tf.StartWorld()

	id, err := cardinal.Create(wCtx, EnergyComponent{}, OwnableComponent{})
	assert.NilError(t, err)
	err = cardinal.UpdateComponent[EnergyComponent](
		wCtx, id, func(component *EnergyComponent) *EnergyComponent {
			component.Amt += 50
			return component
		},
	)
	assert.NilError(t, err)
	err = cardinal.SetComponent[EnergyComponent](
		wCtx, id, &EnergyComponent{
			Amt: 0,
			Cap: 0,
		},
	)
	assert.NilError(t, err)
}

func TestTransactionsSentToRouterAfterTick(t *testing.T) {
	ctrl := gomock.NewController(t)
	rtr := mocks.NewMockRouter(ctrl)
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	world.SetRouter(rtr)
	type fooMsg struct {
		Bar string
	}

	type fooMsgRes struct{}
	fooMessage := message.NewMessageType[fooMsg, fooMsgRes]("foo", message.WithMsgEVMSupport[fooMsg, fooMsgRes]())
	err := cardinal.RegisterMessages(world, fooMessage)
	assert.NilError(t, err)

	evmTxHash := "0x12345"
	msg := fooMsg{Bar: "hello"}
	tx := &sign.Transaction{PersonaTag: "ty"}
	_, txHash := world.AddEVMTransaction(fooMessage.ID(), msg, tx, evmTxHash)

	rtr.
		EXPECT().
		SubmitTxBlob(
			gomock.Any(),
			txpool.TxMap{
				fooMessage.ID(): {
					{
						MsgID:           fooMessage.ID(),
						Msg:             msg,
						TxHash:          txHash,
						Tx:              tx,
						EVMSourceTxHash: evmTxHash,
					},
				},
			},
			world.CurrentTick(),
			gomock.Any(),
		).
		Return(nil).
		Times(1)
	rtr.EXPECT().Start().AnyTimes()
	tf.StartWorld()
	err = world.Tick(context.Background())
	assert.NilError(t, err)
}

// TODO(scott): I commented out this test becase the RecoverFromChain user story doesn't make sense right now.
// RecoverFromChain needs to automatically executed on `StartGame` if:
// 1) router is set
// 2) the current tick (after recovering from redis/memstore) is less than the current tick in the chain

// func TestRecoverFromChain(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	rtr := mocks.NewMockRouter(ctrl)
//	tf := testutils.NewTestFixture(t, nil)
//	world := tf.World
//	world.SetRouter(rtr)
//
//	type fooMsg struct{ I int }
//	type fooMsgRes struct{}
//	fooMsgName := "foo"
//	fooMessage := message.NewMessageType[fooMsg, fooMsgRes](fooMsgName)
//	err := cardinal.RegisterMessages(world, fooMessage)
//	assert.NilError(t, err)
//
//	fooMessages := 0
//	err = cardinal.RegisterSystems(world, func(engineContext cardinal.WorldContext) error {
//		fooMessage.Each(engineContext, func(t message.TxData[fooMsg]) (fooMsgRes, error) {
//			fooMessages++
//			return fooMsgRes{}, nil
//		})
//		return nil
//	})
//	assert.NilError(t, err)
//
//	tf.StartWorld()
//
//	req := &types.QueryTransactionsRequest{
//		Namespace: world.Namespace().String(),
//		Page:      new(types.PageRequest),
//	}
//	msgBody, err := json.Marshal(fooMsg{I: 420})
//	assert.NilError(t, err)
//	tx := &shard.Transaction{
//		PersonaTag: "tyler",
//		Namespace:  world.Namespace().String(),
//		Nonce:      0,
//		Signature:  "sigNature",
//		Body:       msgBody,
//	}
//	bz, err := proto.Marshal(tx)
//	assert.NilError(t, err)
//	pageResponse := &types.PageResponse{Key: []byte("whatever")}
//	res := &types.QueryTransactionsResponse{
//		Epochs: []*types.Epoch{
//			{
//				Epoch:         0,
//				UnixTimestamp: 10,
//				Txs: []*types.Transaction{
//					{
//						TxId:                 uint64(fooMessage.ID()),
//						GameShardTransaction: bz,
//					},
//				},
//			},
//		},
//		Page: pageResponse,
//	}
//	res2 := &types.QueryTransactionsResponse{
//		Epochs: []*types.Epoch{
//			{
//				Epoch:         1,
//				UnixTimestamp: 11,
//				Txs: []*types.Transaction{
//					{
//						TxId:                 uint64(fooMessage.ID()),
//						GameShardTransaction: bz,
//					},
//				},
//			},
//		},
//		Page: nil,
//	}
//	req2 := &types.QueryTransactionsRequest{
//		Namespace: world.Namespace().String(),
//		Page:      &types.PageRequest{Key: pageResponse.Key},
//	}
//	rtr.EXPECT().QueryTransactions(gomock.Any(), req).Return(res, nil).Times(1)
//	rtr.EXPECT().QueryTransactions(gomock.Any(), req2).Return(res2, nil).Times(1)
//
//	err = world.RecoverFromChain(context.Background())
//	assert.NilError(t, err)
//
//	assert.Equal(t, fooMessages, 2)
//	assert.Equal(t, world.CurrentTick(), uint64(2))
// }
