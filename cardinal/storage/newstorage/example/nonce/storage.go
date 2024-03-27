package nonce

import (
	"github.com/redis/go-redis/v9"

	"pkg.world.dev/world-engine/cardinal/storage/newstorage"
	redisstore "pkg.world.dev/world-engine/cardinal/storage/newstorage/redis"
)

type NextNonceStorage interface {
	UseNonce(signerAddress string, nonce uint64) error
}

type nonceStorage struct {
	kv newstorage.KVStore[string, uint64]
}

func NewNonceStorage(opts *redis.Options) NextNonceStorage {
	return &nonceStorage{
		kv: redisstore.NewRedisStorage[string, uint64](opts),
	}
}

func (n *nonceStorage) UseNonce(signerAddress string, nonce uint64) error {
	key := "NONCE-" + signerAddress
	batch := n.kv.NewBatch()

	// Queue a set operation to be executed in a Redis transaction when Commit is called.
	err := batch.Set(key, nonce)
	if err != nil {
		return err
	}

	// Commit the batch to Redis
	err = batch.Commit()
	if err != nil {
		return err
	}

	return nil
}
