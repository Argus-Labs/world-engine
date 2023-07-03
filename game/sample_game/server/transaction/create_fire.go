package transaction

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
)

type CreateFireTransaction struct {
	X, Y int
}

var CreateFire = ecs.NewTransactionType[CreateFireTransaction]()
