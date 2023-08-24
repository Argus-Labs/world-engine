package tx

import "pkg.world.dev/world-engine/cardinal/ecs"

type MoveInput struct {
	Direction string
}

type MoveOutput struct {
	X, Y int64
}

var MoveTx = ecs.NewTransactionType[MoveInput, MoveOutput]("move", ecs.WithTxEVMSupport[MoveInput, MoveOutput])
