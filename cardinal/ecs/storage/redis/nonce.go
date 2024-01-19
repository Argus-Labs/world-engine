package redis

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
)

type NonceStorage struct {
	Client *redis.Client
}

func NewNonceStorage(client *redis.Client) NonceStorage {
	return NonceStorage{
		Client: client,
	}
}

var ErrNonceHasAlreadyBeenUsed = errors.New("nonce has already been used")

// UseNonce atomically marks the given nonce as used. The nonce is valid if nil is returned. A non-nil error means
// there was an error verifying the nonce, or the nonce was already used.
func (r *NonceStorage) UseNonce(signerAddress string, nonce uint64) error {
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
