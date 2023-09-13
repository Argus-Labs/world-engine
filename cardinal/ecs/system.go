package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/encom"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
)

type WorldContext struct {
	World   *World
	ES      *encom.EncomStorage
	TxQueue *transaction.TxQueue
	Logger  *Logger
}

type System func(ctx WorldContext) error
