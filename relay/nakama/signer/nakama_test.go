package signer

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"pkg.world.dev/world-engine/relay/nakama/testutils"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/stretchr/testify/mock"
	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/relay/nakama/mocks"
)

func TestPrivateKeyCanBeLoadedFromDB(t *testing.T) {
	ctx := context.Background()
	logger := testutils.NoopLogger(t)
	nk := mocks.NewNakamaModule(t)
	nonceManager := NewNakamaNonceManager(nk)
	wantPrivateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	storageObj := fmt.Sprintf(`{"Value":"%s"}`, hex.EncodeToString(crypto.FromECDSA(wantPrivateKey)))
	storeReadResult := []*api.StorageObject{
		{
			Value: storageObj,
		},
	}
	nk.On("StorageRead", mock.Anything, testutils.MockMatchReadKey(privateKeyKey)).
		Return(storeReadResult, nil).
		Once()

	txSigner, err := NewNakamaSigner(ctx, logger, nk, nonceManager)
	assert.NilError(t, err)
	gotPrivateKey := txSigner.(*nakamaSigner).privateKey
	assert.Check(t, nil != gotPrivateKey)
	assert.Check(t, gotPrivateKey.Equal(wantPrivateKey))
}

func TestPrivateKeyIsGenerated(t *testing.T) {
	ctx := context.Background()
	logger := testutils.NoopLogger(t)
	nk := mocks.NewNakamaModule(t)
	nonceManager := NewNakamaNonceManager(nk)
	// The DB is checked for an existing private key.
	nk.On("StorageRead", mock.Anything, testutils.MockMatchReadKey(privateKeyKey)).
		Return(nil, nil).
		Once()

	// A newly generated private key is written to the DB.
	nk.On("StorageWrite", mock.Anything, testutils.MockMatchWriteKey(privateKeyKey)).
		Return(nil, nil).
		Once()

	// A new nonce is written to the DB.
	nk.On("StorageWrite", mock.Anything, testutils.MockMatchWriteKey(privateKeyNonce)).
		Return(nil, nil).
		Once()

	txSigner, err := NewNakamaSigner(ctx, logger, nk, nonceManager)
	assert.NilError(t, err)
	gotPrivateKey := txSigner.(*nakamaSigner).privateKey
	assert.Check(t, nil != gotPrivateKey)

	nonceReadResult := []*api.StorageObject{
		{
			Value: `{"Value":"99"}`,
		},
	}

	// To sign a transaction, a nonce will be loaded and incremented
	nk.On("StorageRead", mock.Anything, testutils.MockMatchReadKey(privateKeyNonce)).
		Return(nonceReadResult, nil).
		Once()

	nk.On("StorageWrite", mock.Anything, testutils.MockMatchWriteKey(privateKeyNonce)).
		Return(nil, nil).
		Once()

	payload := map[string]any{"foo": "bar", "baz": "quz"}
	tx, err := txSigner.SignTx(ctx, "foobar", "baz", payload)
	assert.NilError(t, err)
	assert.Equal(t, tx.Nonce, uint64(99))
}
