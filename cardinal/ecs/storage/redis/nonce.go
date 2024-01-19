package redis

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
)

type RedisNonceStorage struct {
	Client *redis.Client
}

func NewRedisNonceStorage(client *redis.Client) RedisNonceStorage {
	return RedisNonceStorage{
		Client: client,
	}
}

var ErrNonceHasAlreadyBeenUsed = errors.New("nonce has already been used")

// UseNonce atomically marks the given nonce as used. The nonce is valid if nil is returned. A non-nil error means
// there was an error verifying the nonce, or the nonce was already used.
func (r *RedisNonceStorage) UseNonce(signerAddress string, nonce uint64) error {
	ctx := context.Background()
	key := r.nonceSetKey(signerAddress)
	added, err := r.Client.SAdd(ctx, key, nonce).Result()
	if err != nil {
		return err
	}
	// The nonce was already used
	if added == 0 {
		return eris.Wrapf(ErrNonceHasAlreadyBeenUsed, "signer %q has already used nonce %d", signerAddress, nonce)
	}
	return nil
}
