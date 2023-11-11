package storage

import (
	"context"
	"errors"
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// Archetypes can just be stored in program memory. It just a structure that allows us to quickly
// decipher combinations of components. There is no point in storing such information in a backend.
// at the very least, we may want to store the list of entities that an archetype has.
//
// Archetype -> group of entities for specific set of components. makes it easy to find entities based on comps.
// example:
//
//
//
// Normal Planet Archetype(1): EnergyComponent, OwnableComponent
// Entities (1), (2), (3)....
//
// In Go memory -> []Archetypes {arch1 (maps to above)}
//
// We need to consider if this needs to be stored in a backend at all. We _should_ be able to build archetypes from
// system restarts as they don't really contain any information about the location of anything stored in a backend.
//
// Something to consider -> we should do something i.e. RegisterComponents, and have it deterministically assign
// TypeID's to components.
// We could end up with some issues (needs to be determined).

type RedisStorage struct {
	Namespace string
	Client    *redis.Client
	Log       zerolog.Logger
}

type Options = redis.Options

func NewRedisStorage(options Options, namespace string) RedisStorage {
	return RedisStorage{
		Namespace: namespace,
		Client:    redis.NewClient(&options),
		Log:       zerolog.New(os.Stdout),
	}
}

// ---------------------------------------------------------------------------
//							Nonce Storage
// ---------------------------------------------------------------------------

var _ NonceStorage = &RedisStorage{}

// GetNonce returns the saved nonce for the given signer address. While signer address will generally be a
// go-ethereum/common.Address, no verification happens at the redis storage level. Any string can be used for the
// signerAddress.
func (r *RedisStorage) GetNonce(signerAddress string) (uint64, error) {
	ctx := context.Background()
	n, err := r.Client.HGet(ctx, r.nonceKey(), signerAddress).Uint64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

// SetNonce saves the given nonce value with the given signer address. Any string can be used for the signer address,
// and no nonce verification takes place.
func (r *RedisStorage) SetNonce(signerAddress string, nonce uint64) error {
	ctx := context.Background()
	return r.Client.HSet(ctx, r.nonceKey(), signerAddress, nonce).Err()
}

func (r *RedisStorage) Close() error {
	err := r.Client.Close()
	if err != nil {
		return err
	}
	return nil
}
