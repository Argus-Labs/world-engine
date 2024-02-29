package cardinal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/router/iterator"
	"pkg.world.dev/world-engine/cardinal/router/mocks"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
	"pkg.world.dev/world-engine/sign"
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
	assert.NilError(t, cardinal.RegisterMessage[FooIn, FooOut](world, "foo", message.WithMsgEVMSupport[FooIn, FooOut]()))
	fooTx, err := cardinal.GetMessageFromWorld[FooIn, FooOut](world)
	assert.NilError(t, err)
	var returnVal FooOut
	var returnErr error
	err = cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			return cardinal.EachMessage[FooIn, FooOut](
				wCtx, func(t message.TxData[FooIn]) (FooOut, error) {
					return returnVal, returnErr
				},
			)
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()

	// add tx to queue
	evmTxHash := "0xFooBar"
	world.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)

	tf.StartWorld()

	// let's check against a system that returns a result and no error
	returnVal = FooOut{Y: "hi"}
	returnErr = nil
	assert.NilError(t, world.Tick(ctx, uint64(time.Now().Unix())))
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
	assert.NilError(t, world.Tick(ctx, uint64(time.Now().Unix())))
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
	err := cardinal.RegisterMessage[FooIn, FooOut](world, "foo", message.WithMsgEVMSupport[FooIn, FooOut]())
	assert.NilError(t, err)

	var returnVal FooOut
	var returnErr error
	err = cardinal.RegisterSystems(world,
		func(eCtx cardinal.WorldContext) error {
			return cardinal.EachMessage[FooIn, FooOut](
				eCtx, func(t message.TxData[FooIn]) (FooOut, error) {
					return returnVal, returnErr
				},
			)
		},
	)
	assert.NilError(t, err)

	tf.StartWorld()

	fooTx, err := cardinal.GetMessageFromWorld[FooIn, FooOut](world)
	assert.NilError(t, err)
	// add tx to queue
	evmTxHash := "0xFooBar"
	world.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)

	// let's check against a system that returns a result and no error
	returnVal = FooOut{Y: "hi"}
	returnErr = nil
	assert.NilError(t, world.Tick(ctx, uint64(time.Now().Unix())))
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
	assert.NilError(t, world.Tick(ctx, uint64(time.Now().Unix())))
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

	err = world.Tick(context.Background(), uint64(time.Now().Unix()))
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
	assert.NilError(t, world.Tick(context.Background(), uint64(time.Now().Unix())))
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
	err := cardinal.RegisterMessage[fooMsg, fooMsgRes](world, "foo", message.WithMsgEVMSupport[fooMsg, fooMsgRes]())
	assert.NilError(t, err)

	evmTxHash := "0x12345"
	msg := fooMsg{Bar: "hello"}
	tx := &sign.Transaction{PersonaTag: "ty"}
	fooMessage, err := cardinal.GetMessageFromWorld[fooMsg, fooMsgRes](world)
	assert.NilError(t, err)
	_, txHash := world.AddEVMTransaction(fooMessage.ID(), msg, tx, evmTxHash)
	ts := uint64(time.Now().Unix())

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
			ts,
		).
		Return(nil).
		Times(1)
	rtr.EXPECT().Start().Times(1)
	rtr.EXPECT().RegisterGameShard(gomock.Any()).Times(1)
	tf.StartWorld()
	err = world.Tick(context.Background(), ts)
	assert.NilError(t, err)

	// Expect that ticks with no transactions are also submitted
	rtr.
		EXPECT().
		SubmitTxBlob(
			gomock.Any(),
			txpool.TxMap{},
			world.CurrentTick(),
			ts,
		).
		Return(nil).
		Times(1)
	rtr.EXPECT().Start().AnyTimes()
	err = world.Tick(context.Background(), ts)
	assert.NilError(t, err)
}

var _ iterator.Iterator = (*FakeIterator)(nil)

// FakeIterator mimics the behavior of a real transaction iterator for testing purposes.
type FakeIterator struct {
	objects []Iterable
}

type Iterable struct {
	Batches   []*iterator.TxBatch
	Tick      uint64
	Timestamp uint64
}

func NewFakeIterator(collection []Iterable) *FakeIterator {
	return &FakeIterator{
		objects: collection,
	}
}

// Each simulates iterating over transactions based on the provided ranges.
// It directly invokes the provided function with mock data for testing.
func (f *FakeIterator) Each(fn func(batch []*iterator.TxBatch, tick, timestamp uint64) error, _ ...uint64) error {
	for _, val := range f.objects {
		// Invoke the callback function with the current batch, tick, and timestamp.
		if err := fn(val.Batches, val.Tick, val.Timestamp); err != nil {
			return err
		}
	}

	return nil
}

// setEnvToCardinalProdMode sets a bunch of environment variables that are required
// for Cardinal to be able to run in Production Mode.
func setEnvToCardinalProdMode(t *testing.T) {
	t.Setenv("CARDINAL_MODE", string(cardinal.RunModeProd))

	t.Setenv("REDIS_ADDRESS", "foo")
	t.Setenv("REDIS_PASSWORD", "bar")
	t.Setenv("CARDINAL_NAMESPACE", "baz")
	t.Setenv("BASE_SHARD_SEQUENCER_ADDRESS", "moo")
	t.Setenv("BASE_SHARD_QUERY_ADDRESS", "oom")
}

func TestRecoverFromChain(t *testing.T) {
	ctrl := gomock.NewController(t)
	rtr := mocks.NewMockRouter(ctrl)
	// Set CARDINAL_MODE to production so that RecoverFromChain() is called
	setEnvToCardinalProdMode(t)

	rtr.EXPECT().Start().Times(1)
	rtr.EXPECT().RegisterGameShard(gomock.Any()).Times(1)

	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	world.SetRouter(rtr)

	type fooMsg struct{ I int }
	type fooMsgRes struct{}
	fooMsgName := "foo"
	assert.NilError(t, cardinal.RegisterMessage[fooMsg, fooMsgRes](world, fooMsgName))

	fooMessages := 0
	err := cardinal.RegisterSystems(world, func(engineContext cardinal.WorldContext) error {
		return cardinal.EachMessage[fooMsg, fooMsgRes](engineContext, func(t message.TxData[fooMsg]) (fooMsgRes, error) {
			fooMessages++
			return fooMsgRes{}, nil
		})
	})
	assert.NilError(t, err)
	fooMessage, err := cardinal.GetMessageFromWorld[fooMsg, fooMsgRes](world)
	assert.NilError(t, err)
	fakeBatches := []Iterable{
		{
			Batches: []*iterator.TxBatch{
				{
					MsgID:    fooMessage.ID(),
					MsgValue: fooMsg{I: 1},
					Tx:       &sign.Transaction{},
				},
				{
					MsgID:    fooMessage.ID(),
					MsgValue: fooMsg{I: 2},
					Tx:       &sign.Transaction{},
				},
			},
			Tick:      1,
			Timestamp: uint64(time.Now().Unix()),
		},
		{
			Batches: []*iterator.TxBatch{
				{
					MsgID:    fooMessage.ID(),
					MsgValue: fooMsg{I: 3},
					Tx:       &sign.Transaction{},
				},
				{
					MsgID:    fooMessage.ID(),
					MsgValue: fooMsg{I: 4},
					Tx:       &sign.Transaction{},
				},
			},
			Tick:      15,
			Timestamp: uint64(time.Now().Unix()),
		},
	}

	fakeIterator := NewFakeIterator(fakeBatches)

	rtr.EXPECT().TransactionIterator().Return(fakeIterator).Times(1)
	tf.StartWorld()

	// fooMessages should have been incremented 4 times for each of the 4 txs
	assert.Equal(t, fooMessages, 4)
	// World should be ready for tick 16
	assert.Equal(t, world.CurrentTick(), uint64(16))
}
