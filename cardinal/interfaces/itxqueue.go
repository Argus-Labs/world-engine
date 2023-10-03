package interfaces

import (
	"pkg.world.dev/world-engine/sign"
)

type ITxQueue interface {
	GetAmountOfTxs() int
	AddTransaction(id TransactionTypeID, v any, sig *sign.SignedPayload) TxHash
	CopyTransaction() ITxQueue
	ForID(id TransactionTypeID) []TxAny
}
