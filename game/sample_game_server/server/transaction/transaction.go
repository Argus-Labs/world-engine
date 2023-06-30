package transaction

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

type MoveTransaction struct {
	ID storage.EntityID
	XDelta, YDelta int
}

type CreatePlayerTransaction struct {
	X, Y int
}

type CreateFireTransaction struct {
	X, Y int
}

var (
	Move         = ecs.NewTransactionType[MoveTransaction]()
	CreatePlayer = ecs.NewTransactionType[CreatePlayerTransaction]()
	CreateFire   = ecs.NewTransactionType[CreateFireTransaction]()
)

func MustInitialize(world *ecs.World) {
	err := world.RegisterTransactions(
		Move,
		CreatePlayer,
		CreateFire,
	)
	if err != nil {
		panic(err)
	}
}
