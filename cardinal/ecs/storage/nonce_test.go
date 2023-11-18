package storage_test

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/internal/ecstestutils"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestSetAndGetNonce(t *testing.T) {
	rs := ecstestutils.GetRedisStorage(t)
	address := "some-address"
	wantNonce := uint64(100)
	testutils.AssertNilErrorWithTrace(t, rs.SetNonce(address, wantNonce))
	gotNonce, err := rs.GetNonce(address)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, gotNonce, wantNonce)
}

func TestMissingNonceIsZero(t *testing.T) {
	rs := ecstestutils.GetRedisStorage(t)

	gotNonce, err := rs.GetNonce("some-address-that-doesn't-exist")
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, uint64(0), gotNonce)
}

func TestCanStoreManyNonces(t *testing.T) {
	rs := ecstestutils.GetRedisStorage(t)
	for i := uint64(10); i < 100; i++ {
		addr := fmt.Sprintf("%d", i)
		testutils.AssertNilErrorWithTrace(t, rs.SetNonce(addr, i))
	}

	for i := uint64(10); i < 100; i++ {
		addr := fmt.Sprintf("%d", i)
		gotNonce, err := rs.GetNonce(addr)
		testutils.AssertNilErrorWithTrace(t, err)
		assert.Equal(t, i, gotNonce)
	}
}
