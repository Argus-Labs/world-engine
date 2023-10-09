package main

// private_key.go manages the creation and loading of the Nakama private key used to sign
// all transactions.

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/heroiclabs/nakama-common/runtime"
)

var (
	ErrorNoStorageObjectFound       = errors.New("no storage object found")
	ErrorTooManyStorageObjectsFound = errors.New("too many storage objects found")

	// globalPrivateKey stores Nakama's private key, so it does not have to be periodically fetched from the StorageObject
	// system. This pattern should have a security pass. See:
	// https://linear.app/arguslabs/issue/NAK-5/review-the-pattern-of-storing-the-nakama-private-key-in-a-global
	globalPrivateKey    *ecdsa.PrivateKey
	globalSignerAddress string
	// nonceMutex is used to ensure that Nakama's current nonce can be read and incremented atomically.
	nonceMutex sync.Mutex
)

const (
	privateKeyCollection = "private_key_collection"
	privateKeyKey        = "private_key_key"
	privateKeyNonce      = "private_key_nonce"
	adminAccountID       = "00000000-0000-0000-0000-000000000000"
)

type privateKeyStorageObj struct {
	Value string
}

// getOnePKStorageObj loads one specific runtime.StorageObject from the privateKeyCollection in
// Nakama's storage layer. An error is returned if too few or too many storage objects are found.
func getOnePKStorageObj(ctx context.Context, nk runtime.NakamaModule, key string) (string, error) {
	objs, err := nk.StorageRead(ctx, []*runtime.StorageRead{{
		Collection: privateKeyCollection,
		UserID:     adminAccountID,
		Key:        key,
	}})
	if err != nil {
		return "", err
	}
	if len(objs) > 1 {
		return "", ErrorTooManyStorageObjectsFound
	} else if len(objs) == 0 {
		return "", ErrorNoStorageObjectFound
	}
	var pkObj privateKeyStorageObj
	if err := json.Unmarshal([]byte(objs[0].Value), &pkObj); err != nil {
		return "", err
	}
	return pkObj.Value, nil
}

// setOnePKStorageObj saves the given value to the privateKeyCollection in Nakama's storage layer.
func setOnePKStorageObj(ctx context.Context, nk runtime.NakamaModule, key, value string) error {
	pkObj := privateKeyStorageObj{
		Value: value,
	}
	buf, err := json.Marshal(pkObj)
	if err != nil {
		return err
	}
	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{{
		Collection:      privateKeyCollection,
		UserID:          adminAccountID,
		Key:             key,
		Value:           string(buf),
		Version:         "",
		PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
		PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
	}})
	return err
}

func getPrivateKeyHex(ctx context.Context, nk runtime.NakamaModule) (string, error) {
	return getOnePKStorageObj(ctx, nk, privateKeyKey)
}

func setPrivateKeyHex(ctx context.Context, nk runtime.NakamaModule, hex string) error {
	return setOnePKStorageObj(ctx, nk, privateKeyKey, hex)
}

func getNonce(ctx context.Context, nk runtime.NakamaModule) (uint64, error) {
	value, err := getOnePKStorageObj(ctx, nk, privateKeyNonce)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(value, 10, 64)
}

func setNonce(ctx context.Context, nk runtime.NakamaModule, n uint64) error {
	return setOnePKStorageObj(ctx, nk, privateKeyNonce, fmt.Sprintf("%d", n))
}

// initPrivateKey either reads the existing private key form the nakama DB, or generates a new private key if one
// does not exist.
func initPrivateKey(ctx context.Context, logger runtime.Logger, nk runtime.NakamaModule) error {
	privateKeyHex, err := getPrivateKeyHex(ctx, nk)
	if err != nil {
		if err != ErrorNoStorageObjectFound {
			return fmt.Errorf("failed to get private key: %w", err)
		}
		logger.Debug("no private key found; creating a new one")
		// No private key found. Let's generate one.
		privateKey, err := crypto.GenerateKey()
		if err != nil {
			return err
		}
		privateKeyHex = hex.EncodeToString(crypto.FromECDSA(privateKey))
		if err := setPrivateKeyHex(ctx, nk, privateKeyHex); err != nil {
			return err
		}
		if err := setNonce(ctx, nk, 1); err != nil {
			return err
		}
	}
	// We've either loaded the existing private key, or initialized a new one
	globalPrivateKey, err = crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return err
	}
	globalSignerAddress = crypto.PubkeyToAddress(globalPrivateKey.PublicKey).Hex()
	return nil
}

func getSignerAddress() string {
	return globalSignerAddress
}

func incrementNonce(ctx context.Context, nk runtime.NakamaModule) (nonce uint64, err error) {
	nonceMutex.Lock()
	defer nonceMutex.Unlock()
	nonce, err = getNonce(ctx, nk)
	if err != nil {
		return 0, err
	}
	newNonce := nonce + 1
	if err := setNonce(ctx, nk, newNonce); err != nil {
		return 0, err
	}
	return nonce, nil
}

// getPrivateKeyAndANonce returns the global Nakama private key, as well as a unique nonce that can
// be used to sign a transaction. The nonce is guaranteed to be unique, and this method is safe
// for concurrent access.
func getPrivateKeyAndANonce(ctx context.Context, nk runtime.NakamaModule) (*ecdsa.PrivateKey, uint64, error) {
	nonce, err := incrementNonce(ctx, nk)
	if err != nil {
		return nil, 0, err
	}
	return globalPrivateKey, nonce, nil
}
