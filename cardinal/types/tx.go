package types

import (
	"github.com/ethereum/go-ethereum/common"

	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

type TxMap = map[string][]TxData

type TxData struct {
	Tx *sign.Transaction
	// Msg needs to be seperately serialized because it can be either ABI-encoded or JSON-encoded.
	Msg message.Message
	// EVMSourceTxHash is the tx hash of the EVM tx that triggered this tx.
	EVMSourceTxHash *common.Hash
}
