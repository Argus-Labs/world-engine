package storage_test

import (
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/cardinal/storage/redis"
	"testing"

	"pkg.world.dev/world-engine/assert"
)

const Namespace string = "world"

func GetRedisStorage(t *testing.T) redis.Storage {
	s := miniredis.RunT(t)
	return redis.NewRedisStorage(redis.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, Namespace)
}
func TestUseNonce(t *testing.T) {
	rs := GetRedisStorage(t)
	address := "some-address"
	nonce := uint64(100)
	assert.NilError(t, rs.UseNonce(address, nonce))
}

func TestCanStoreManyNonces(t *testing.T) {
	rs := GetRedisStorage(t)
	for i := uint64(10); i < 100; i++ {
		addr := fmt.Sprintf("%d", i)
		assert.NilError(t, rs.UseNonce(addr, i))
	}

	// These nonces can no longer be used
	for i := uint64(10); i < 100; i++ {
		addr := fmt.Sprintf("%d", i)
		err := rs.UseNonce(addr, i)
		assert.ErrorIs(t, redis.ErrNonceHasAlreadyBeenUsed, err)
	}
}
