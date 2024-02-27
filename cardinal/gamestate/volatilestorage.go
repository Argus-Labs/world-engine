package gamestate

// this interface is meant for in memory storage
type VolatileStorage[K comparable, V any] interface {
	Get(key K) (V, error)
	Set(key K, value V) error
	Delete(key K) error
	Keys() ([]K, error)
	Clear() error
}
