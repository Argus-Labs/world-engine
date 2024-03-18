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
	nonceManager := NewNakamaNonceManager(fakeNK)

	// A private key should be generated and saved in fakeNK
	firstTxSigner, err := NewNakamaSigner(ctx, logger, fakeNK, nonceManager)
	assert.NilError(t, err)
	firstSignerAddress := firstTxSigner.SignerAddress()
	assert.NotEmpty(t, firstSignerAddress)

	// Since we're reusing fakeNK, the previously stored private key should be found.
	secondTxSigner, err := NewNakamaSigner(ctx, logger, fakeNK, nonceManager)
	assert.NilError(t, err)
	secondSignerAddress := secondTxSigner.SignerAddress()
	assert.NotEmpty(t, secondSignerAddress)

	// If the first and second signer addresses match, it means the private keys are the same.
	assert.Equal(t, firstSignerAddress, secondSignerAddress)

	// Generating a new signer, with a brand new (empty) nakama storage layer should generate a new key
	emptyNK := testutils.NewFakeNakamaModule()
	thirdTxSigner, err := NewNakamaSigner(ctx, logger, emptyNK, nonceManager)
	assert.NilError(t, err)
	thirdSignerAddress := thirdTxSigner.SignerAddress()

	// This new key's signer address should not match the first or second.
	assert.NotEqual(t, firstSignerAddress, thirdSignerAddress)
}

func TestNonceIsIncremented(t *testing.T) {
	ctx := context.Background()
	logger := testutils.MockNoopLogger(t)
	nk := testutils.NewFakeNakamaModule()
	nonceManager := NewNakamaNonceManager(nk)

	txSigner, err := NewNakamaSigner(ctx, logger, nk, nonceManager)
	assert.NilError(t, err)
	gotPrivateKey := txSigner.(*nakamaSigner).privateKey
	assert.Check(t, nil != gotPrivateKey)

	// The nonce should increment each time we sign a transaction
	for wantNonce := 1; wantNonce <= 10; wantNonce++ {
		payload := map[string]any{"foo": "bar", "baz": "quz"}
		tx, err := txSigner.SignTx(ctx, "foobar", "baz", payload)
		assert.NilError(t, err)
		assert.Equal(t, tx.Nonce, uint64(wantNonce))
	}
}
