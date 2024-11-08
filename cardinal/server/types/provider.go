package types

import (
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/server/sign"
	"pkg.world.dev/world-engine/cardinal/server/validator"
	"pkg.world.dev/world-engine/cardinal/types"
)

type ProviderWorld interface {
	validator.SignerAddressProvider
	UseNonce(signerAddress string, nonce uint64) error
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
	Namespace() string
	GetComponentByName(name string) (types.ComponentMetadata, error)
	StoreReader() gamestate.Reader
	HandleQuery(group string, name string, bz []byte) ([]byte, error)
	CurrentTick() uint64
	ReceiptHistorySize() uint64
	GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
	EvaluateCQL(cql string) ([]types.EntityStateElement, error)
	GetDebugState() ([]types.DebugStateElement, error)
	BuildQueryFields() []types.FieldDetail
}
