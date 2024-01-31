package engine

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

type Context interface {
	Timestamp() uint64
	CurrentTick() uint64
	Logger() *zerolog.Logger
	// SetLogger is used to inject a new logger configuration to an engine context that is already created.
	SetLogger(logger zerolog.Logger)
	GetComponentByName(name string) (component.ComponentMetadata, error)
	AddMessageError(id message.TxHash, err error)
	SetMessageResult(id message.TxHash, a any)
	GetTransactionReceipt(id message.TxHash) (any, []error, bool)
	EmitEvent(string)
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
	ReceiptHistorySize() uint64
	Namespace() string
	ListQueries() []Query
	ListMessages() []message.Message
	AddTransaction(id message.TypeID, v any, sig *sign.Transaction) (uint64, message.TxHash)
	UseNonce(signerAddress string, nonce uint64) error

	// For internal use.
	StoreReader() gamestate.Reader
	StoreManager() gamestate.Manager
	GetTxQueue() *txpool.TxQueue
	IsReadOnly() bool
}
