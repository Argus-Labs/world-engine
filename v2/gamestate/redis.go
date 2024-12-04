package gamestate

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var _ PrimitiveStorage[string] = &RedisStorage{}

type RedisStorage struct {
	currentClient redis.Cmdable
	tracer        trace.Tracer
}

func NewRedisPrimitiveStorage(client redis.Cmdable) PrimitiveStorage[string] {
	return &RedisStorage{
		currentClient: client,
		tracer:        otel.Tracer("redis"),
	}
}

func (r *RedisStorage) GetFloat64(ctx context.Context, key string) (float64, error) {
	res, err := r.currentClient.Get(ctx, key).Float64()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}
func (r *RedisStorage) GetFloat32(ctx context.Context, key string) (float32, error) {
	res, err := r.currentClient.Get(ctx, key).Float32()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}
func (r *RedisStorage) GetUInt64(ctx context.Context, key string) (uint64, error) {
	res, err := r.currentClient.Get(ctx, key).Uint64()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}

func (r *RedisStorage) GetInt64(ctx context.Context, key string) (int64, error) {
	res, err := r.currentClient.Get(ctx, key).Int64()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}

func (r *RedisStorage) GetInt(ctx context.Context, key string) (int, error) {
	res, err := r.currentClient.Get(ctx, key).Int()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}

func (r *RedisStorage) GetBool(ctx context.Context, key string) (bool, error) {
	res, err := r.currentClient.Get(ctx, key).Bool()
	if err != nil {
		return false, eris.Wrap(err, "")
	}
	return res, nil
}

func (r *RedisStorage) GetBytes(ctx context.Context, key string) ([]byte, error) {
	bz, err := r.currentClient.Get(ctx, key).Bytes()
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return bz, nil
}

func (r *RedisStorage) Set(ctx context.Context, key string, value any) error {
	return eris.Wrap(r.currentClient.Set(ctx, key, value, 0).Err(), "")
}

// Underlying type is a string. Unfortunately this is the way redis works and this is the most generic return value.
func (r *RedisStorage) Get(ctx context.Context, key string) (any, error) {
	var res any
	var err error
	res, err = r.currentClient.Get(ctx, key).Result()
	return res, eris.Wrap(err, "")
}

func (r *RedisStorage) Incr(ctx context.Context, key string) error {
	return eris.Wrap(r.currentClient.Incr(ctx, key).Err(), "")
}

func (r *RedisStorage) Decr(ctx context.Context, key string) error {
	return eris.Wrap(r.currentClient.Decr(ctx, key).Err(), "")
}

func (r *RedisStorage) Delete(ctx context.Context, key string) error {
	return eris.Wrap(r.currentClient.Del(ctx, key).Err(), "")
}

func (r *RedisStorage) Close(ctx context.Context) error {
	return eris.Wrap(r.currentClient.Shutdown(ctx).Err(), "")
}

func (r *RedisStorage) Keys(ctx context.Context) ([]string, error) {
	return r.currentClient.Keys(ctx, "*").Result()
}

func (r *RedisStorage) Clear(ctx context.Context) error {
	return eris.Wrap(r.currentClient.FlushAll(ctx).Err(), "")
}

func (r *RedisStorage) StartTransaction(_ context.Context) (PrimitiveStorage[string], error) {
	pipeline := r.currentClient.TxPipeline()
	redisTransaction := NewRedisPrimitiveStorage(pipeline)
	return redisTransaction, nil
}

func (r *RedisStorage) EndTransaction(ctx context.Context) error {
	ctx, span := r.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "redis.transaction.end")
	defer span.End()

	pipeline, ok := r.currentClient.(redis.Pipeliner)
	if !ok {
		err := eris.New("current redis dbStorage is not a pipeline/transaction")
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return err
	}

	_, err := pipeline.Exec(ctx)
	if err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return err
	}

	return nil
}
