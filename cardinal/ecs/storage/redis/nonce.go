package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
)

type NonceStorage interface {
	GetNonce(key string) (uint64, error)
	SetNonce(key string, nonce uint64) error
}

// GetNonce returns the saved nonce for the given signer address. While signer address will generally be a
// go-ethereum/common.Address, no verification happens at the redis storage level. Any string can be used for the
// signerAddress.
func (r *Storage) GetNonce(signerAddress string) (uint64, error) {
	ctx := context.Background()
	n, err := r.Client.HGet(ctx, r.nonceKey(), signerAddress).Uint64()
	err = eris.Wrap(err, "")
	if err != nil {
		if eris.Is(eris.Cause(err), redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

// SetNonce saves the given nonce value with the given signer address. Any string can be used for the signer address,
// and no nonce verification takes place.
func (r *Storage) SetNonce(signerAddress string, nonce uint64) error {
	ctx := context.Background()
	return eris.Wrap(r.Client.HSet(ctx, r.nonceKey(), signerAddress, nonce).Err(), "")
}
