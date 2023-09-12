package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
)

type System func(*World, *transaction.TxQueue, *Logger) error
