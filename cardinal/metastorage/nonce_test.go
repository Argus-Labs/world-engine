package metastorage_test

import (
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/cardinal/metastorage/redis"

	"pkg.world.dev/world-engine/assert"
)

const Namespace string = "world"

func GetRedisStorage(t *testing.T) redis.MetaStorage {
	s := miniredis.RunT(t)
	return redis.NewRedisMetaStorage(redis.Options{
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
