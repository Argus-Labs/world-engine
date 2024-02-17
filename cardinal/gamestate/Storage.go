package gamestate

import (
	"context"
)

// PrimitiveStorage is the interface for all available stores related to the game loop
// there is another store like interface for other logistical values located in `ecs.metastorage`
type PrimitiveStorage interface {
	GetFloat64(ctx context.Context, key string) (float64, error)
	GetFloat32(ctx context.Context, key string) (float32, error)
	GetUInt64(ctx context.Context, key string) (uint64, error)
	GetInt64(ctx context.Context, key string) (int64, error)
	GetInt(ctx context.Context, key string) (int, error)
	GetBool(ctx context.Context, key string) (bool, error)
	GetBytes(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value any) error
	Incr(ctx context.Context, key string) error
	Decr(ctx context.Context, key string) error
	Delete(ctx context.Context, key string) error
	StartTransaction(ctx context.Context) (Transaction, error)
	EndTransaction(ctx context.Context) error
	Close(ctx context.Context) error
}

type Transaction = PrimitiveStorage
