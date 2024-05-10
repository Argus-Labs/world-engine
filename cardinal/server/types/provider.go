package types

import (
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/search"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/sign"
)

type ProviderWorld interface {
	UseNonce(signerAddress string, nonce uint64) error
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
	Namespace() string
	GetComponentByName(name string) (types.ComponentMetadata, error)
	Search(filter filter.ComponentFilter) search.EntitySearch
	StoreReader() gamestate.Reader
	QueryHandler(name string, group string, bz []byte) ([]byte, error)
	CurrentTick() uint64
	ReceiptHistorySize() uint64
	GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
	RunCQLSearch(filter filter.ComponentFilter) ([]types.CqlData, error, error)
	GetDebugState() (types.DebugStateResponse, error, error)
	BuildQueryFields() []engine.FieldDetail
}
