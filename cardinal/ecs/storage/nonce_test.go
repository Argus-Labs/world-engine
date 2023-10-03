package storage_test

import (
	"fmt"
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/testutil"

	"gotest.tools/v3/assert"
)

func TestSetAndGetNonce(t *testing.T) {
	rs := testutil.GetRedisStorage(t)
	address := "some-address"
	wantNonce := uint64(100)
	assert.NilError(t, rs.SetNonce(address, wantNonce))
	gotNonce, err := rs.GetNonce(address)
	assert.NilError(t, err)
	assert.Equal(t, gotNonce, wantNonce)
}

func TestMissingNonceIsZero(t *testing.T) {
	rs := testutil.GetRedisStorage(t)

	gotNonce, err := rs.GetNonce("some-address-that-doesn't-exist")
	assert.NilError(t, err)
	assert.Equal(t, uint64(0), gotNonce)
}

func TestCanStoreManyNonces(t *testing.T) {
	rs := testutil.GetRedisStorage(t)
	for i := uint64(10); i < 100; i++ {
		addr := fmt.Sprintf("%d", i)
		assert.NilError(t, rs.SetNonce(addr, i))
	}

	for i := uint64(10); i < 100; i++ {
		addr := fmt.Sprintf("%d", i)
		gotNonce, err := rs.GetNonce(addr)
		assert.NilError(t, err)
		assert.Equal(t, i, gotNonce)
	}
}
