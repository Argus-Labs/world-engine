package signer

import (
	"context"
	"encoding/hex"
	"fmt"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/stretchr/testify/mock"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/relay/nakama/mocks"
)

func TestPrivateKeyCanBeLoadedFromDB(t *testing.T) {
	assert.Check(t, nil == globalPrivateKey, "unable to test private key generation; a key has already been generated")
	t.Cleanup(func() {
		globalPrivateKey = nil
	})
	wantPrivateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	storageObj := fmt.Sprintf(`{"Value":"%s"}`, hex.EncodeToString(crypto.FromECDSA(wantPrivateKey)))
	storeReadResult := []*api.StorageObject{
		{
			Value: storageObj,
		},
	}
	nk := mocks.NewNakamaModule(t)
	nk.On("StorageRead", mock.Anything, testutils.MockMatchReadKey(privateKeyKey)).
		Return(storeReadResult, nil).
		Once()

	err = InitPrivateKey(context.Background(), testutils.NoopLogger(t), nk)
	assert.NilError(t, err)
	assert.Check(t, nil != globalPrivateKey)
	assert.Check(t, globalPrivateKey.Equal(wantPrivateKey))
}

func TestPrivateKeyIsGenerated(t *testing.T) {
	assert.Check(t, nil == globalPrivateKey, "unable to test private key generation; a key has already been generated")
	t.Cleanup(func() {
		globalPrivateKey = nil
	})
	nk := mocks.NewNakamaModule(t)
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

	err := InitPrivateKey(context.Background(), testutils.NoopLogger(t), nk)
	assert.NilError(t, err)
	assert.Check(t, nil != globalPrivateKey)
}
