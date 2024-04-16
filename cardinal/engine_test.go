package cardinal_test

import (
	"errors"
	"strconv"
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
	type FooIn struct {
		X uint32
	}
	type FooOut struct {
		Y string
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	msgName := "foo"
	assert.NilError(t,
		cardinal.RegisterMessage[FooIn, FooOut](world, msgName, message.WithMsgEVMSupport[FooIn, FooOut]()))
	fooTx, ok := world.GetMessageByFullName("game." + msgName)
	assert.True(t, ok)
	var returnVal FooOut
	var returnErr error
	err := cardinal.RegisterSystems(
		world,
		func(wCtx engine.Context) error {
			return cardinal.EachMessage[FooIn, FooOut](
				wCtx, func(message.TxData[FooIn]) (FooOut, error) {
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
	tf.DoTick()
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
	tf.DoTick()
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
	type FooIn struct {
		X uint32
	}
	type FooOut struct {
		Y string
	}
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	msgName := "foo"
	err := cardinal.RegisterMessage[FooIn, FooOut](world, msgName, message.WithMsgEVMSupport[FooIn, FooOut]())
	assert.NilError(t, err)

	var returnVal FooOut
	var returnErr error
	err = cardinal.RegisterSystems(world,
		func(eCtx cardinal.WorldContext) error {
			return cardinal.EachMessage[FooIn, FooOut](
				eCtx, func(message.TxData[FooIn]) (FooOut, error) {
					return returnVal, returnErr
				},
			)
		},
	)
	assert.NilError(t, err)

	tf.StartWorld()

	fooTx, ok := world.GetMessageByFullName("game." + msgName)
	assert.True(t, ok)
	// add tx to queue
	evmTxHash := "0xFooBar"
	world.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)

	// let's check against a system that returns a result and no error
	returnVal = FooOut{Y: "hi"}
	returnErr = nil
	tf.DoTick()
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
	tf.DoTick()
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

	tf.DoTick()

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
	tf.DoTick()
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
	assert.Equal(t, world.Namespace(), namespace)
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
	tf := testutils.NewTestFixture(t, nil, cardinal.WithCustomRouter(rtr))
	world := tf.World

	type fooMsg struct {
		Bar string
	}

	type fooMsgRes struct{}

	msgName := "foo"
	err := cardinal.RegisterMessage[fooMsg, fooMsgRes](world, msgName, message.WithMsgEVMSupport[fooMsg, fooMsgRes]())
	assert.NilError(t, err)

	evmTxHash := "0x12345"
	msg := fooMsg{Bar: "hello"}
	tx := &sign.Transaction{PersonaTag: "ty"}
	fooMessage, ok := world.GetMessageByFullName("game." + msgName)
	assert.True(t, ok)
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
	tf.DoTick()

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
	tf.DoTick()
}

// setEnvToCardinalRollupMode sets a bunch of environment variables that are required
// for Cardinal to be able to run in rollup node.
func setEnvToCardinalRollupMode(t *testing.T) {
	t.Setenv("CARDINAL_ROLLUP_ENABLED", strconv.FormatBool(true))
	t.Setenv("BASE_SHARD_ROUTER_KEY", "77cf59146831dbd94bd19dd4b259b268ee07a7c1fdba67e92b0f7c1cfdfb7a9b")
}

func TestRecoverFromChain(t *testing.T) {
	ctrl := gomock.NewController(t)
	rtr := mocks.NewMockRouter(ctrl)

	// Set CARDINAL_ROLLUP_ENABLED=true so that RecoverFromChain() is called
	setEnvToCardinalRollupMode(t)

	rtr.EXPECT().Start().Times(1)
	rtr.EXPECT().RegisterGameShard(gomock.Any()).Times(1)

	tf := testutils.NewTestFixture(t, nil, cardinal.WithCustomRouter(rtr))
	world := tf.World

	type fooMsg struct{ I int }
	type fooMsgRes struct{}
	fooMsgName := "foo"
	assert.NilError(t, cardinal.RegisterMessage[fooMsg, fooMsgRes](world, fooMsgName))

	fooMessages := 0
	err := cardinal.RegisterSystems(world, func(engineContext cardinal.WorldContext) error {
		return cardinal.EachMessage[fooMsg, fooMsgRes](engineContext, func(message.TxData[fooMsg]) (fooMsgRes, error) {
			fooMessages++
			return fooMsgRes{}, nil
		})
	})
	assert.NilError(t, err)
	fooMessage, ok := world.GetMessageByFullName("game." + fooMsgName)
	assert.True(t, ok)
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
