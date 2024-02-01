package cardinal_test

import (
	"context"
	"encoding/json"
	"errors"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/shard/adapter"
	types2 "pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/evm/x/shard/types"

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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	// Make sure the game can tick
	tf.StartWorld()
	tf.DoTick()

	assert.NilError(t, world.Shutdown())

	for i := 0; i < 10; i++ {
		// After a engine is shut down, WaitForNextTick should never block and always fail
		assert.Check(t, !world.WaitForNextTick())
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
	world := testutils.NewTestFixture(t, nil).World
	fooTx := message.NewMessageType[FooIn, FooOut]("foo", message.WithMsgEVMSupport[FooIn, FooOut]())
	assert.NilError(t, cardinal.RegisterMessages(world, fooTx))
	var returnVal FooOut
	var returnErr error
	err := cardinal.RegisterSystems(
		world,
		func(wCtx types2.Context) error {
			fooTx.Each(
				wCtx, func(t message.TxData[FooIn]) (FooOut, error) {
					return returnVal, returnErr
				},
			)
			return nil
		},
	)
	assert.NilError(t, err)
	assert.NilError(t, world.LoadGameState())

	// add tx to queue
	evmTxHash := "0xFooBar"
	world.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)

	// let's check against a system that returns a result and no error
	returnVal = FooOut{Y: "hi"}
	returnErr = nil
	assert.NilError(t, world.Tick(ctx))
	evmTxReceipt, ok := world.ConsumeEVMMsgResult(evmTxHash)
	assert.Equal(t, ok, true)
	assert.Check(t, len(evmTxReceipt.ABIResult) > 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 0)
	// shouldn't be able to consume it again.
	_, ok = world.ConsumeEVMMsgResult(evmTxHash)
	assert.Equal(t, ok, false)

	// lets check against a system that returns an error
	returnVal = FooOut{}
	returnErr = errors.New("omg error")
	world.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)
	assert.NilError(t, world.Tick(ctx))
	evmTxReceipt, ok = world.ConsumeEVMMsgResult(evmTxHash)

	assert.Equal(t, ok, true)
	assert.Equal(t, len(evmTxReceipt.ABIResult), 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 1)
	// shouldn't be able to consume it again.
	_, ok = world.ConsumeEVMMsgResult(evmTxHash)
	assert.Equal(t, ok, false)
}

func TestAddSystems(t *testing.T) {
	count := 0
	sys1 := func(types2.Context) error {
		count++
		return nil
	}
	sys2 := func(types2.Context) error {
		count++
		return nil
	}
	sys3 := func(types2.Context) error {
		count++
		return nil
	}

	world := testutils.NewTestFixture(t, nil).World
	err := cardinal.RegisterSystems(world, sys1, sys2, sys3)
	assert.NilError(t, err)

	err = world.LoadGameState()
	assert.NilError(t, err)

	err = world.Tick(context.Background())
	assert.NilError(t, err)

	assert.Equal(t, count, 3)
}

func TestSystemExecutionOrder(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	order := make([]int, 0, 3)
	err := cardinal.RegisterSystems(
		world,
		func(types2.Context) error {
			order = append(order, 1)
			return nil
		}, func(types2.Context) error {
			order = append(order, 2)
			return nil
		}, func(types2.Context) error {
			order = append(order, 3)
			return nil
		},
	)
	assert.NilError(t, err)
	err = world.LoadGameState()
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
	world := testutils.NewTestFixture(t, nil).World
	assert.Equal(t, world.Namespace().String(), namespace)
}

func TestWithoutRegistration(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
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
	assert.NilError(t, world.LoadGameState())

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

type dummyAdapter struct {
	txs           txpool.TxMap
	ns            string
	epoch         uint64
	unixTimestamp uint64
}

func (d *dummyAdapter) Submit(
	_ context.Context,
	txs txpool.TxMap,
	namespace string,
	epoch, unixTimestamp uint64,
) error {
	d.txs = txs
	d.ns = namespace
	d.epoch = epoch
	d.unixTimestamp = unixTimestamp
	return nil
}

func (d *dummyAdapter) QueryTransactions(_ context.Context, _ *types.QueryTransactionsRequest) (
	*types.QueryTransactionsResponse, error,
) {
	return &types.QueryTransactionsResponse{}, nil
}

var _ adapter.Adapter = &dummyAdapter{}

type TestStruct struct {
}

// TestAdapterCalledAfterTick tests that when messages are executed in a tick, they are forwarded to the adapter.
func TestAdapterCalledAfterTick(t *testing.T) {
	adapter := &dummyAdapter{}
	tf := testutils.NewTestFixture(t, nil, cardinal.WithAdapter(adapter))
	world := tf.World

	err := cardinal.RegisterSystems(world, func(engineContext types2.Context) error {
		return nil
	})
	assert.NilError(t, err)
	fooMessage := message.NewMessageType[struct{}, struct{}]("foo")
	err = cardinal.RegisterMessages(world, fooMessage)
	assert.NilError(t, err)
	err = world.LoadGameState()
	assert.NilError(t, err)

	tf.AddTransaction(fooMessage.ID(), fooMessage, &sign.Transaction{
		PersonaTag: "meow",
		Namespace:  "foo",
		Nonce:      22,
		Signature:  "meow",
		Hash:       common.Hash{},
		Body:       json.RawMessage(`{}`),
	})
	tf.AddTransaction(fooMessage.ID(), fooMessage, &sign.Transaction{
		PersonaTag: "meow",
		Namespace:  "foo",
		Nonce:      23,
		Signature:  "meow",
		Hash:       common.Hash{},
		Body:       json.RawMessage(`{}`),
	})
	err = world.Tick(context.Background())
	assert.NilError(t, err)

	assert.Len(t, adapter.txs[fooMessage.ID()], 2)
	assert.Equal(t, world.Namespace().String(), adapter.ns)
	assert.Equal(t, world.CurrentTick()-1, adapter.epoch)
}
