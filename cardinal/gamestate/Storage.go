package gamestate

import (
	"context"
)

// PrimitiveStorage is the interface for all available stores related to the game loop
// there is another store like interface for other logistical values located in `ecs.metastorage`
type PrimitiveStorage[K comparable] interface {
	GetFloat64(ctx context.Context, key K) (float64, error)
	GetFloat32(ctx context.Context, key K) (float32, error)
	GetUInt64(ctx context.Context, key K) (uint64, error)
	GetInt64(ctx context.Context, key K) (int64, error)
	GetInt(ctx context.Context, key K) (int, error)
	GetBool(ctx context.Context, key K) (bool, error)
	GetBytes(ctx context.Context, key K) ([]byte, error)
	Set(ctx context.Context, key K, value any) error
	Incr(ctx context.Context, key K) error
	Decr(ctx context.Context, key K) error
	Delete(ctx context.Context, key K) error
	StartTransaction(ctx context.Context) (Transaction[K], error)
	EndTransaction(ctx context.Context) error
	Close(ctx context.Context) error
	Keys(ctx context.Context) ([]K, error)
}

type Transaction[K comparable] interface {
	PrimitiveStorage[K]
}
