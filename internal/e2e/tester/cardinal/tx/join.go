package tx

import (
	"pkg.world.dev/world-engine/cardinal"
)

type JoinInput struct {
	Player string `json:"player"`
}

type JoinOutput struct{}

var JoinTx = cardinal.NewTransactionTypeWithEVMSupport[JoinInput, JoinOutput]("join")
