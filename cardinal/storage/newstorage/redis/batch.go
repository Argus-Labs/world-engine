package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/storage/newstorage"
)

var _ newstorage.KVBatch[any, any] = &batch[any, any]{}
var _ newstorage.KVReadBatch[any, any] = &batch[any, any]{}

type batch[K comparable, V any] struct {
	pipe        redis.Pipeliner
	isDiscarded bool
}

// IsDiscarded returns true if the batch has been discarded.
// No more operations can be performed on a discarded batch.
func (r *batch[K, V]) IsDiscarded() bool {
	return r.isDiscarded
}

// Has returns true if the key exists in the Redis store.
// As a part of the batch, this operation receives transaction isolation guarantees.
// That means dirty reads are not possible.
func (r *batch[K, V]) Has(_ K) (bool, error) {
	if err := r.checkBatchNotDiscarded(); err != nil {
		return false, err
	}
	return false, eris.New("not implemented")
}

// Get obtains the value of a key from the Redis store.
// As a part of the batch, this operation receives transaction isolation guarantees.
// That means dirty reads are not possible.
func (r *batch[K, V]) Get(key K) (*V, error) {
	if err := r.checkBatchNotDiscarded(); err != nil {
		return nil, err
	}

	value, err := r.pipe.Do(context.Background(), "get", fmt.Sprintf("%v", key)).Result()
	if err != nil {
		return nil, err
	}
	return value.(*V), nil
}

// Set queues a set operation to be executed in a Redis transaction when Commit is called.
func (r *batch[K, V]) Set(key K, value V) error {
	if err := r.checkBatchNotDiscarded(); err != nil {
		return err
	}

	_, err := r.pipe.Set(context.Background(), fmt.Sprintf("%v", key), value, 0).Result()
	if err != nil {
		return err
	}
	return nil
}

// Delete queues a delete operation to be executed in a Redis transaction when Commit is called.
func (r *batch[K, V]) Delete(_ K) error {
	if err := r.checkBatchNotDiscarded(); err != nil {
		return err
	}
	return eris.New("not implemented")
}

// Discard discards the batch and prevents it from being used anymore
func (r *batch[K, V]) Discard() error {
	if err := r.checkBatchNotDiscarded(); err != nil {
		return err
	}

	r.pipe.Discard()
	r.isDiscarded = true
	return nil
}

// Commit the tranasction to redis
func (r *batch[K, V]) Commit() error {
	if err := r.checkBatchNotDiscarded(); err != nil {
		return err
	}

	// Commit the transaction
	_, err := r.pipe.Exec(context.Background())
	if err != nil {
		return err
	}

	// Mark the batch as discarded so it can't be used anymore
	r.isDiscarded = true

	return nil
}

func (r *batch[K, V]) checkBatchNotDiscarded() error {
	if r.isDiscarded {
		return eris.New("batch is already discarded")
	}
	return nil
}
