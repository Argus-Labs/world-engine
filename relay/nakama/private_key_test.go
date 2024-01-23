package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/stretchr/testify/mock"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/relay/nakama/mocks"
)

func mockMatchReadKey(key string) interface{} {
	return mock.MatchedBy(func(storeRead []*runtime.StorageRead) bool {
		if len(storeRead) != 1 {
			return false
		}
		return storeRead[0].Key == key
	})
}

func mockMatchWriteKey(key string) interface{} {
	return mock.MatchedBy(func(storeWrite []*runtime.StorageWrite) bool {
		if len(storeWrite) != 1 {
			return false
		}
		return storeWrite[0].Key == key
	})
}

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
	nk.On("StorageRead", mock.Anything, mockMatchReadKey(privateKeyKey)).
		Return(storeReadResult, nil).
		Once()

	err = initPrivateKey(context.Background(), NoopLogger(t), nk)
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
	nk.On("StorageRead", mock.Anything, mockMatchReadKey(privateKeyKey)).
		Return(nil, nil).
		Once()

	// A newly generated private key is written to the DB.
	nk.On("StorageWrite", mock.Anything, mockMatchWriteKey(privateKeyKey)).
		Return(nil, nil).
		Once()

	// A new nonce is written to the DB.
	nk.On("StorageWrite", mock.Anything, mockMatchWriteKey(privateKeyNonce)).
		Return(nil, nil).
		Once()

	err := initPrivateKey(context.Background(), NoopLogger(t), nk)
	assert.NilError(t, err)
	assert.Check(t, nil != globalPrivateKey)
}
