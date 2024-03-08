package engine

import (
	"reflect"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
	"pkg.world.dev/world-engine/sign"
)

//go:generate mockgen -source=context.go -package mocks -destination=mocks/context.go
type Context interface {
	// Timestamp returns the UNIX timestamp of the tick.
	Timestamp() uint64
	// CurrentTick returns the current tick.
	CurrentTick() uint64
	// Logger returns the logger that can be used to log messages from within system or query.
	Logger() *zerolog.Logger
	// EmitEvent emits an event that will be broadcasted to all websocket subscribers.
	EmitEvent(map[string]any) error
	// Namespace returns the namespace of the world.
	Namespace() string

	// For internal use.

	// SetLogger is used to inject a new logger configuration to an engine context that is already created.
	SetLogger(logger zerolog.Logger)
	AddMessageError(id types.TxHash, err error)
	SetMessageResult(id types.TxHash, a any)
	GetComponentByName(name string) (types.ComponentMetadata, error)
	GetMessageByType(mType reflect.Type) (types.Message, bool)
	GetTransactionReceipt(id types.TxHash) (any, []error, bool)
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
	ReceiptHistorySize() uint64
	AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
	UseNonce(signerAddress string, nonce uint64) error
	IsWorldReady() bool
	StoreReader() gamestate.Reader
	StoreManager() gamestate.Manager
	GetTxPool() *txpool.TxPool
	IsReadOnly() bool
}
