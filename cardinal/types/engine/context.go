package engine

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

type Context interface {
	Timestamp() uint64
	CurrentTick() uint64
	Logger() *zerolog.Logger
	// SetLogger is used to inject a new logger configuration to an engine context that is already created.
	SetLogger(logger zerolog.Logger)
	GetComponentByName(name string) (types.ComponentMetadata, error)
	AddMessageError(id types.TxHash, err error)
	SetMessageResult(id types.TxHash, a any)
	GetTransactionReceipt(id types.TxHash) (any, []error, bool)
	EmitEvent(string)
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
	ReceiptHistorySize() uint64
	Namespace() string
	AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
	UseNonce(signerAddress string, nonce uint64) error

	// For internal use.
	StoreReader() gamestate.Reader
	StoreManager() gamestate.Manager
	GetTxQueue() *txpool.TxQueue
	IsReadOnly() bool
}
