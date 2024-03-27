package newstorage

// KVStore is a generic key-value store interface with batch operations.
type KVStore[K comparable, V any] interface {
	// KVStore exposed a batch system for write operations.
	// Where it executes multiple operations in a single transaction.

	NewBatch() KVBatch[K, V]
	NewReadBatch() KVReadBatch[K, V]

	// KVStore also provides a direct reader and writer that
	// executes a single operation at a time.

	kvReader[K, V]
	kvWriter[K, V]

	// Close closes the underlying connection to the store.
	Close() error
}

// KVBatch is an abstraction interface for a batch of KVStore operations.
// This interface must have transaction isolation guarantees to avoid dirty reads and deadlocks.
// KVBatch is needed for write operations.
type KVBatch[K comparable, V any] interface {
	kvReader[K, V]
	kvWriter[K, V]
	IsDiscarded() bool
	Discard() error
	Commit() error
}

// KVReadBatch is an abstraction interface for a batch of KVStore read operations.
// This interface must have transaction isolation guarantees to avoid dirty reads .
// KVBatch is preferred when you only need to read from the store.
type KVReadBatch[K comparable, V any] interface {
	kvReader[K, V]
	Discard() error
}

type kvReader[K comparable, V any] interface {
	Has(key K) (bool, error)
	Get(key K) (*V, error)
}

type kvWriter[K comparable, V any] interface {
	Set(key K, value V) error
	Delete(key K) error
}
