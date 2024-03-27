package redis

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/storage/newstorage"
)

var _ newstorage.KVStore[string, any] = &Store[string, any]{}

type Store[K comparable, V any] struct {
	client *redis.Client
}

func NewRedisStorage[K comparable, V any](opts *redis.Options) *Store[K, V] {
	return &Store[K, V]{
		client: redis.NewClient(opts),
	}
}

func (r *Store[K, V]) NewBatch() newstorage.KVBatch[K, V] {
	return &batch[K, V]{
		pipe:        r.client.TxPipeline(),
		isDiscarded: false,
	}
}

func (r *Store[K, V]) NewReadBatch() newstorage.KVReadBatch[K, V] {
	return &batch[K, V]{
		pipe:        r.client.TxPipeline(),
		isDiscarded: false,
	}
}

func (r *Store[K, V]) Has(_ K) (bool, error) {
	return false, eris.New("not implemented")
}

func (r *Store[K, V]) Get(key K) (*V, error) {
	value, err := r.client.Do(context.Background(), "get", fmt.Sprintf("%v", key)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, eris.Wrap(err, "key does not exists")
		}
		return nil, err
	}
	return value.(*V), nil
}

func (r *Store[K, V]) Set(key K, value V) error {
	_, err := r.client.Set(context.Background(), fmt.Sprintf("%v", key), value, 0).Result()
	if err != nil {
		return err
	}
	return nil
}

func (r *Store[K, V]) Delete(_ K) error {
	return eris.New("not implemented")
}

func (r *Store[K, V]) Close() error {
	err := r.client.Close()
	if err != nil {
		return err
	}
	return nil
}
