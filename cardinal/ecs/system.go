package ecs

import (
	"pkg.world.dev/world-engine/cardinal/engine/log"
	"pkg.world.dev/world-engine/cardinal/engine/transaction"
)

type System func(*World, *transaction.TxQueue, *log.Logger) error
