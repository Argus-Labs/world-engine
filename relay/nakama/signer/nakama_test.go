package signer

import (
	"context"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
)

func TestPrivateKeyCanBeLoadedFromDB(t *testing.T) {
	ctx := context.Background()
	logger := testutils.MockNoopLogger(t)
	fakeNK := testutils.NewFakeNakamaModule()

	// A private key should be generated and saved in fakeNK
	firstTxSigner, err := NewNakamaSigner(ctx, logger, fakeNK)
	assert.NilError(t, err)
	firstSignerAddress := firstTxSigner.SignerAddress()
	assert.NotEmpty(t, firstSignerAddress)

	// Since we're reusing fakeNK, the previously stored private key should be found.
	secondTxSigner, err := NewNakamaSigner(ctx, logger, fakeNK)
	assert.NilError(t, err)
	secondSignerAddress := secondTxSigner.SignerAddress()
	assert.NotEmpty(t, secondSignerAddress)

	// If the first and second signer addresses match, it means the private keys are the same.
	assert.Equal(t, firstSignerAddress, secondSignerAddress)

	// Generating a new signer, with a brand new (empty) nakama storage layer should generate a new key
	emptyNK := testutils.NewFakeNakamaModule()
	thirdTxSigner, err := NewNakamaSigner(ctx, logger, emptyNK)
	assert.NilError(t, err)
	thirdSignerAddress := thirdTxSigner.SignerAddress()

	// This new key's signer address should not match the first or second.
	assert.NotEqual(t, firstSignerAddress, thirdSignerAddress)
}
