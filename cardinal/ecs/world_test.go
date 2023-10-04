package ecs_test

import (
	"context"
	"pkg.world.dev/world-engine/cardinal/engine"
	"pkg.world.dev/world-engine/cardinal/engine/log"
	"pkg.world.dev/world-engine/cardinal/engine/transaction"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
)

func TestAddSystems(t *testing.T) {
	count := 0
	sys := func(w *ecs.World, txq *transaction.TxQueue, _ *log.Logger) error {
		count++
		return nil
	}

	w := ecs.NewTestWorld(t)
	w.AddSystems(sys, sys, sys)
	err := w.LoadGameState()
	assert.NilError(t, err)

	err = w.Tick(context.Background())
	assert.NilError(t, err)

	assert.Equal(t, count, 3)
}

func TestSystemExecutionOrder(t *testing.T) {
	w := ecs.NewTestWorld(t)
	order := make([]int, 0, 3)
	w.AddSystems(func(world *ecs.World, queue *transaction.TxQueue, logger *log.Logger) error {
		order = append(order, 1)
		return nil
	}, func(world *ecs.World, queue *transaction.TxQueue, logger *log.Logger) error {
		order = append(order, 2)
		return nil
	}, func(world *ecs.World, queue *transaction.TxQueue, logger *log.Logger) error {
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
	w := ecs.NewTestWorld(t, engine.WithNamespace(id))
	assert.Equal(t, w.Namespace(), id)
}

func TestWithoutRegistration(t *testing.T) {
	world := ecs.NewTestWorld(t)
	id, err := world.Create(Energy, Ownable)
	assert.Assert(t, err != nil)

	err = Energy.Update(world, id, func(component EnergyComponent) EnergyComponent {
		component.Amt += 50
		return component
	})
	assert.Assert(t, err != nil)

	err = Energy.Set(world, id, EnergyComponent{
		Amt: 0,
		Cap: 0,
	})

	assert.Assert(t, err != nil)

	err = world.RegisterComponents(Energy, Ownable)
	assert.NilError(t, err)
	id, err = world.Create(Energy, Ownable)
	assert.NilError(t, err)
	err = Energy.Update(world, id, func(component EnergyComponent) EnergyComponent {
		component.Amt += 50
		return component
	})
	assert.NilError(t, err)
	err = Energy.Set(world, id, EnergyComponent{
		Amt: 0,
		Cap: 0,
	})
	assert.NilError(t, err)

}
