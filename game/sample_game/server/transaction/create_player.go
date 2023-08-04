package transaction

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
)

type CreatePlayerTransaction struct {
	X, Y int
}

var CreatePlayer = ecs.NewTransactionType[CreatePlayerTransaction]("create-player", false)
