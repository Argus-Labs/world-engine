package ecs_test

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

type SendEnergyTxResult struct{}

type EnergyComp struct {
	value int
}

var energy = ecs.NewComponentType[EnergyComp]()
