package public

import (
	"context"
	"time"

	"pkg.world.dev/world-engine/cardinal/shard"
	"pkg.world.dev/world-engine/sign"
)

type IWorld interface {
	IsRecovering() bool
	StoreManager() IStoreManager
	GetTxQueueAmount() int
	AddSystem(s System)
	AddSystems(s ...System)
	RegisterComponents(components ...IComponentType) error
	GetComponentByName(name string) (IComponentType, bool)
	RegisterReads(reads ...IRead) error
	RegisterTransactions(txs ...ITransaction) error
	ListReads() []IRead
	ListTransactions() ([]ITransaction, error)
	IsGameLoopRunning() bool
	LoadGameState() error
	RecoverFromChain(ctx context.Context) error
	Namespace() string
	SetNamespace(namespace string)
	GetNonce(signerAddress string) (uint64, error)
	SetNonce(signerAddress string, nonce uint64) error
	GetChain() *shard.ReadAdapter
	SetChain(chain *shard.Adapter)
	AddTransactionError(id TxHash, err error)
	SetTransactionResult(id TxHash, a any)
	GetTransactionReceipt(id TxHash) (any, []error, bool)
	GetTransactionReceiptsForTick(tick uint64) ([]IReceipt, error)
	AddTransaction(id TransactionTypeID, v any, sig *sign.SignedPayload) (tick uint64, txHash TxHash)
	Tick(ctx context.Context) error
	CurrentTick() uint64
	ReceiptHistorySize() uint64
	SetReceiptHistory(history IHistory)
	CreateMany(num int, components ...IComponentType) ([]EntityID, error)
	// Len return the number of entities in this world
	EntityAmount() (int, error)
	Remove(id EntityID) error
	StartGameLoop(ctx context.Context, tickStart <-chan time.Time, tickDone chan<- uint64)
	EndGameLoop()
	GetComponents() []IComponentType
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	GetSystemNames() []string
	LogError(err error)
	GetLogger() IWorldLogger
	InjectLogger(logger IWorldLogger)
	Create(components ...IComponentType) (EntityID, error)
	GetComponentFromName(name string) (IComponentType, bool)
	//ILoggable
}
