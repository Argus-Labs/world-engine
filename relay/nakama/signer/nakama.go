package signer

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/sign"
)

const (
	privateKeyCollection = "private_key_collection"
	privateKeyKey        = "private_key_key"
)

var _ Signer = &nakamaSigner{}

var (
	ErrNoStorageObjectFound       = errors.New("no storage object found")
	ErrTooManyStorageObjectsFound = errors.New("too many storage objects found")
)

type privateKeyStorageObj struct {
	Value string
}

type nakamaSigner struct {
	nk            runtime.NakamaModule
	privateKey    *ecdsa.PrivateKey
	signerAddress string
	nonceManager  NonceManager
}

func (n *nakamaSigner) SignTx(
	ctx context.Context,
	personaTag string,
	namespace string,
	data any,
) (tx *sign.Transaction, err error) {
	nonce, err := n.nonceManager.IncNonce(ctx)
	if err != nil {
		return nil, eris.Wrap(err, "failed to increment nonce")
	}

	if personaTag == "" {
		tx, err = sign.NewSystemTransaction(n.privateKey, namespace, nonce, data)
	} else {
		tx, err = sign.NewTransaction(n.privateKey, personaTag, namespace, nonce, data)
	}

	if err != nil {
		return nil, eris.Wrap(err, "failed to sign transaction")
	}
	return tx, nil
}

func (n *nakamaSigner) SignSystemTx(ctx context.Context, namespace string, data any) (*sign.Transaction, error) {
	return n.SignTx(ctx, "", namespace, data)
}

func (n *nakamaSigner) SignerAddress() string {
	return n.signerAddress
}

func NewNakamaSigner(ctx context.Context, logger runtime.Logger, nk runtime.NakamaModule, nonceManager NonceManager) (
	Signer, error,
) {
	privateKeyHex, err := getPrivateKeyHex(ctx, nk)
	if err != nil {
		if !eris.Is(eris.Cause(err), ErrNoStorageObjectFound) {
			return nil, eris.Wrap(err, "failed to get private key")
		}
		logger.Debug("no private key found; creating a new one")
		// No private key found. Let's generate one.
		var privateKey *ecdsa.PrivateKey
		privateKey, err = crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		privateKeyHex = hex.EncodeToString(crypto.FromECDSA(privateKey))
		if err = setPrivateKeyHex(ctx, nk, privateKeyHex); err != nil {
			return nil, err
		}
		if err = nonceManager.SetNonce(ctx, 1); err != nil {
			return nil, err
		}
	}
	// We've either loaded the existing private key, or initialized a new one
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	signerAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	return &nakamaSigner{
		nk:            nk,
		privateKey:    privateKey,
		signerAddress: signerAddress,
		nonceManager:  nonceManager,
	}, nil
}

func getPrivateKeyHex(ctx context.Context, nk runtime.NakamaModule) (string, error) {
	return getOnePKStorageObj(ctx, nk, privateKeyKey)
}

// getOnePKStorageObj loads one specific runtime.StorageObject from the privateKeyCollection in
// Nakama's storage layer. An error is returned if too few or too many storage objects are found.
func getOnePKStorageObj(ctx context.Context, nk runtime.NakamaModule, key string) (string, error) {
	objs, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{
			Collection: privateKeyCollection,
			UserID:     AdminAccountID,
			Key:        key,
		},
	})
	if err != nil {
		return "", eris.Wrap(err, "")
	}
	if len(objs) > 1 {
		return "", eris.Wrap(ErrTooManyStorageObjectsFound, "")
	} else if len(objs) == 0 {
		return "", eris.Wrap(ErrNoStorageObjectFound, "")
	}
	var pkObj privateKeyStorageObj
	if err = json.Unmarshal([]byte(objs[0].GetValue()), &pkObj); err != nil {
		return "", eris.Wrap(err, "")
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
		return eris.Wrap(err, "")
	}
	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{
		{
			Collection:      privateKeyCollection,
			UserID:          AdminAccountID,
			Key:             key,
			Value:           string(buf),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		},
	})
	return err
}

func setPrivateKeyHex(ctx context.Context, nk runtime.NakamaModule, hex string) error {
	return setOnePKStorageObj(ctx, nk, privateKeyKey, hex)
}
