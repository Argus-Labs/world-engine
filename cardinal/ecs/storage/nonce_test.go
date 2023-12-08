package storage_test

import (
	"fmt"
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"pkg.world.dev/world-engine/cardinal/ecs/storage/redis"

	"pkg.world.dev/world-engine/assert"
)

func TestUseNonce(t *testing.T) {
	rs := testutil.GetRedisStorage(t)
	address := "some-address"
	nonce := uint64(100)
	assert.NilError(t, rs.Nonce.UseNonce(address, nonce))
}

func TestCanStoreManyNonces(t *testing.T) {
	rs := testutil.GetRedisStorage(t)
	for i := uint64(10); i < 100; i++ {
		addr := fmt.Sprintf("%d", i)
		assert.NilError(t, rs.Nonce.UseNonce(addr, i))
	}

	// These nonces can no longer be used
	for i := uint64(10); i < 100; i++ {
		addr := fmt.Sprintf("%d", i)
		err := rs.Nonce.UseNonce(addr, i)
		assert.ErrorIs(t, redis.ErrNonceHasAlreadyBeenUsed, err)
	}
}
