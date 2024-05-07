package types

import (
	"reflect"

	"github.com/rs/zerolog"

	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/search"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
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
	//GetReadOnlyCtx() engine.Context
}

type ProviderQuery interface {
	Name() string
	HandleQueryRaw(wCtx ProviderContext, bz []byte) ([]byte, error)
	GetRequestFieldInformation() map[string]any
	Group() string
}

type ProviderContext interface {
	CurrentTick() uint64
	ReceiptHistorySize() uint64
	GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
	Namespace() string
	Timestamp() uint64
	Logger() *zerolog.Logger
	EmitEvent(map[string]any) error
	EmitStringEvent(string) error
	SetLogger(logger zerolog.Logger)
	AddMessageError(id types.TxHash, err error)
	SetMessageResult(id types.TxHash, a any)
	GetComponentByName(name string) (types.ComponentMetadata, error)
	GetMessageByType(mType reflect.Type) (types.Message, bool)
	GetTransactionReceipt(id types.TxHash) (any, []error, bool)
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
	IsWorldReady() bool
	StoreReader() gamestate.Reader
	StoreManager() gamestate.Manager
	GetTxPool() *txpool.TxPool
	IsReadOnly() bool
}
