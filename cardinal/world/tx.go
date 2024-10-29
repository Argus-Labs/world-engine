package world

import (
	"github.com/ethereum/go-ethereum/common"

	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

type Tx[Msg message.Message] struct {
	Hash common.Hash
	Msg  Msg
	Tx   *sign.Transaction
}
