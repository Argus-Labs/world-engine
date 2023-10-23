package tx

import (
	"pkg.world.dev/world-engine/cardinal"
)

type MoveInput struct {
	Direction string
}

type MoveOutput struct {
	X, Y int64
}

var MoveTx = cardinal.NewTransactionTypeWithEVMSupport[MoveInput, MoveOutput]("move")
