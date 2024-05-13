package types

import (
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

type ProviderWorld interface {
	UseNonce(signerAddress string, nonce uint64) error
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
	Namespace() string
	GetComponentByName(name string) (types.ComponentMetadata, error)
	StoreReader() gamestate.Reader
	QueryHandler(name string, bz []byte) ([]byte, error)
	CurrentTick() uint64
	ReceiptHistorySize() uint64
	GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
	EvaluateCQL(cql string) ([]types.CqlData, error, error)
	GetDebugState() (types.DebugStateResponse, error)
	BuildQueryFields() []types.FieldDetail
}

// GetWorldResponse is a type representing the json super structure that contains
// all info about the world.
type GetWorldResponse struct {
	Namespace  string              `json:"namespace"`
	Components []types.FieldDetail `json:"components"` // list of component names
	Messages   []types.FieldDetail `json:"messages"`
	Queries    []types.FieldDetail `json:"queries"`
}
