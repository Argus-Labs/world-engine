package ecs_test

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

func TestAddSystems(t *testing.T) {
	count := 0
	sys := func(ecs.WorldContext) error {
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
	w.AddSystems(func(ecs.WorldContext) error {
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
