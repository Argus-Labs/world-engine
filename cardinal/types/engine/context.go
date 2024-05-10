package engine

//go:generate mockgen -source=context.go -package mocks -destination=mocks/context.go
// type Context interface {
//	// Timestamp returns the UNIX timestamp of the tick.
//	Timestamp() uint64
//	// CurrentTick returns the current tick.
//	CurrentTick() uint64
//	// Logger returns the logger that can be used to log messages from within system or query.
//	Logger() *zerolog.Logger
//	// EmitEvent emits an event that will be broadcast to all websocket subscribers.
//	EmitEvent(map[string]any) error
//	// EmitStringEvent emits a string event that will be broadcast to all websocket subscribers.
//	// This method is provided for backwards compatability. EmitEvent should be used for most cases.
//	EmitStringEvent(string) error
//	// Namespace returns the namespace of the world.
//	Namespace() string
//
//	// For internal use.
//
//	// setLogger is used to inject a new logger configuration to an engine context that is already created.
//	setLogger(logger zerolog.Logger)
//	addMessageError(id types.TxHash, err error)
//	setMessageResult(id types.TxHash, a any)
//	GetComponentByName(name string) (types.ComponentMetadata, error)
//	getMessageByType(mType reflect.Type) (types.Message, bool)
//	getTransactionReceipt(id types.TxHash) (any, []error, bool)
//	getSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
//	getTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
//	receiptHistorySize() uint64
//	addTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
//	isWorldReady() bool
//	storeReader() gamestate.Reader
//	storeManager() gamestate.Manager
//	getTxPool() *txpool.TxPool
//	isReadOnly() bool
//}
