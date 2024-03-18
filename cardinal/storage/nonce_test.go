package storage_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/storage/redis"
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

func TestNonceStorageIsBounded(t *testing.T) {
	rs := GetRedisStorage(t)
	addr := "some-address"
	// totalNonceCount is the total number of nonces we want to "use"
	totalNonceCount := 3 * redis.NonceSlidingWindowSize
	// maxNonceSaved is the maximum number of nonces we should expect to see at the end of this test. If this value is
	// exceeded it likely means the storage for nonces is unbounded. We don't want to set this limit to *exactly*
	// NonceSlidingWindowSize because we want the nonce manager to have some leeway as to when exactly it will prune
	// old nonces.
	maxNonceSaved := 2 * redis.NonceSlidingWindowSize
	for i := 0; i < totalNonceCount; i++ {
		assert.NilError(t, rs.UseNonce(addr, uint64(i)), "using nonce %v failed", i)
	}

	// Find the redis key that contains the used nonces for the above address
	client := rs.Client
	ctx := context.Background()
	keys, err := client.Keys(ctx, "*"+addr+"*").Result()
	assert.NilError(t, err)
	assert.Equal(t, 1, len(keys))

	// This test assumes we're using Redis to keep track of used nonces. If/when our storage layer changes, this test
	// should be kept, but the mechanism for counting the number of saved nonces will need to be updated.
	count, err := client.ZCard(ctx, keys[0]).Result()
	assert.NilError(t, err)
	assert.Check(t, count < int64(maxNonceSaved), "nonce tracking is unbounded")
}

func TestOutOfOrderNoncesAreOK(t *testing.T) {
	count := 100
	nonces := make([]uint64, count)
	for i := range nonces {
		nonces[i] = uint64(i)
	}
	r := rand.New(rand.NewSource(0))
	r.Shuffle(count, func(i, j int) {
		nonces[i], nonces[j] = nonces[j], nonces[i]
	})

	rs := GetRedisStorage(t)
	for _, n := range nonces {
		assert.NilError(t, rs.UseNonce("some-addr", n))
	}
	m := map[int]int{}
	clear(m)
}

// TestCannotReuseNonceAfterPrune ensures off-by-one errors related to pruning nonces outside the NonceSlidingWindowSize
// do not result in the ability to reuse an already-used nonce.
func TestCannotReuseNonceAfterPrune(t *testing.T) {
	rs := GetRedisStorage(t)
	total := 3 * redis.NonceSlidingWindowSize
	addr := "some-addr"
	for i := 0; i < total; i++ {
		assert.NilError(t, rs.UseNonce(addr, uint64(i)))
		if i > 10 {
			err := rs.UseNonce(addr, uint64(i-10))
			assert.ErrorIs(t, redis.ErrNonceHasAlreadyBeenUsed, err)
		}
		if i > redis.NonceSlidingWindowSize+1 {
			alreadyUsed := uint64(i - redis.NonceSlidingWindowSize)
			before := alreadyUsed - 1
			after := alreadyUsed + 1

			// Make sure the nonces around the sliding window size always return an error
			assert.IsError(t, rs.UseNonce(addr, before), "%d was already used", before)
			assert.IsError(t, rs.UseNonce(addr, alreadyUsed), "%d was already used", alreadyUsed)
			assert.IsError(t, rs.UseNonce(addr, after), "%d was already used", after)
		}
	}
}

func TestUsedNoncesAreRememberedAcrossRestart(t *testing.T) {
	s := miniredis.RunT(t)
	opts := redis.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}
	rsOne := redis.NewRedisStorage(opts, Namespace)

	addr := "some-addr"
	for i := 0; i < 10; i++ {
		assert.NilError(t, rsOne.UseNonce(addr, uint64(i)))
	}

	rsTwo := redis.NewRedisStorage(opts, Namespace)
	for i := 0; i < 10; i++ {
		err := rsTwo.UseNonce(addr, uint64(i))
		assert.ErrorIs(t, redis.ErrNonceHasAlreadyBeenUsed, err)
	}
}
