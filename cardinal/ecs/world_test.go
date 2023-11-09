package ecs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"pkg.world.dev/world-engine/sign"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

func TestWaitForNextTick(t *testing.T) {
	w := ecs.NewTestWorld(t)
	doneCh := make(chan struct{})
	go func() {
		tick := w.CurrentTick()
		w.WaitForNextTick()
		assert.Check(t, w.CurrentTick() > tick)
		doneCh <- struct{}{}
	}()
	assert.NilError(t, w.LoadGameState())
	tickDone := make(chan uint64)
	w.StartGameLoop(context.Background(), time.Tick(100*time.Millisecond), tickDone)
	<-doneCh
}

func TestEVMTxConsume(t *testing.T) {
	ctx := context.Background()
	type FooIn struct {
		X uint32
	}
	type FooOut struct {
		Y string
	}
	w := ecs.NewTestWorld(t)
	fooTx := ecs.NewMessageType[FooIn, FooOut]("foo", ecs.WithMsgEVMSupport[FooIn, FooOut])
	assert.NilError(t, w.RegisterMessages(fooTx))
	var returnVal FooOut
	var returnErr error
	w.RegisterSystem(func(wCtx ecs.WorldContext) error {
		fooTx.ForEach(wCtx, func(t ecs.TxData[FooIn]) (FooOut, error) {
			return returnVal, returnErr
		})
		return nil
	})
	assert.NilError(t, w.LoadGameState())

	// add tx to queue
	evmTxHash := "0xFooBar"
	w.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)

	// let's check against a system that returns a result and no error
	returnVal = FooOut{Y: "hi"}
	returnErr = nil
	assert.NilError(t, w.Tick(ctx))
	evmTxReceipt, ok := w.ConsumeEVMMsgResult(evmTxHash)
	assert.Equal(t, ok, true)
	assert.Check(t, len(evmTxReceipt.ABIResult) > 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 0)
	// shouldn't be able to consume it again.
	_, ok = w.ConsumeEVMMsgResult(evmTxHash)
	assert.Equal(t, ok, false)

	// lets check against a system that returns an error
	returnVal = FooOut{}
	returnErr = errors.New("omg error")
	w.AddEVMTransaction(fooTx.ID(), FooIn{X: 32}, &sign.Transaction{PersonaTag: "foo"}, evmTxHash)
	assert.NilError(t, w.Tick(ctx))
	evmTxReceipt, ok = w.ConsumeEVMMsgResult(evmTxHash)

	assert.Equal(t, ok, true)
	assert.Equal(t, len(evmTxReceipt.ABIResult), 0)
	assert.Equal(t, evmTxReceipt.EVMTxHash, evmTxHash)
	assert.Equal(t, len(evmTxReceipt.Errs), 1)
	// shouldn't be able to consume it again.
	_, ok = w.ConsumeEVMMsgResult(evmTxHash)
	assert.Equal(t, ok, false)
}

func TestAddSystems(t *testing.T) {
	count := 0
	sys := func(ecs.WorldContext) error {
		count++
		return nil
	}

	w := ecs.NewTestWorld(t)
	w.RegisterSystems(sys, sys, sys)
	err := w.LoadGameState()
	assert.NilError(t, err)

	err = w.Tick(context.Background())
	assert.NilError(t, err)

	assert.Equal(t, count, 3)
}

func TestSystemExecutionOrder(t *testing.T) {
	w := ecs.NewTestWorld(t)
	order := make([]int, 0, 3)
	w.RegisterSystems(func(ecs.WorldContext) error {
		order = append(order, 1)
		return nil
	}, func(ecs.WorldContext) error {
		order = append(order, 2)
		return nil
	}, func(ecs.WorldContext) error {
		order = append(order, 3)
		return nil
	})
	err := w.LoadGameState()
	assert.NilError(t, err)
	assert.NilError(t, w.Tick(context.Background()))
	expectedOrder := []int{1, 2, 3}
	for i, elem := range order {
		assert.Equal(t, elem, expectedOrder[i])
	}
}

func TestSetNamespace(t *testing.T) {
	id := "foo"
	w := ecs.NewTestWorld(t, ecs.WithNamespace(id))
	assert.Equal(t, w.Namespace().String(), id)
}

func TestWithoutRegistration(t *testing.T) {
	world := ecs.NewTestWorld(t)
	wCtx := ecs.NewWorldContext(world)
	id, err := component.Create(wCtx, EnergyComponent{}, OwnableComponent{})
	assert.Assert(t, err != nil)

	err = component.UpdateComponent[EnergyComponent](wCtx, id, func(component *EnergyComponent) *EnergyComponent {
		component.Amt += 50
		return component
	})
	assert.Assert(t, err != nil)

	err = component.SetComponent[EnergyComponent](wCtx, id, &EnergyComponent{
		Amt: 0,
		Cap: 0,
	})

	assert.Assert(t, err != nil)

	assert.NilError(t, ecs.RegisterComponent[EnergyComponent](world))
	assert.NilError(t, ecs.RegisterComponent[OwnableComponent](world))
	id, err = component.Create(wCtx, EnergyComponent{}, OwnableComponent{})
	assert.NilError(t, err)
	err = component.UpdateComponent[EnergyComponent](wCtx, id, func(component *EnergyComponent) *EnergyComponent {
		component.Amt += 50
		return component
	})
	assert.NilError(t, err)
	err = component.SetComponent[EnergyComponent](wCtx, id, &EnergyComponent{
		Amt: 0,
		Cap: 0,
	})
	assert.NilError(t, err)
}
