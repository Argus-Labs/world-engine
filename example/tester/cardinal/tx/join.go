package tx

import "pkg.world.dev/world-engine/cardinal/ecs"

type JoinInput struct {
}

type JoinOutput struct{}

var JoinTx = ecs.NewTransactionType[JoinInput, JoinOutput]("join", ecs.WithTxEVMSupport[JoinInput, JoinOutput])
