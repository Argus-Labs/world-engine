package transaction

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

type MoveTransaction struct {
	ID             storage.EntityID
	XDelta, YDelta int
}

var Move = ecs.NewTransactionType[MoveTransaction]("move", false)
