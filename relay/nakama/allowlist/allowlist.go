package allowlist

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	nakamaerrors "pkg.world.dev/world-engine/relay/nakama/errors"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"strings"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

var (
	EnabledEnvVar = "ENABLE_ALLOWLIST"
	Enabled       = false
	KeyCollection = "allowlist_keys_collection"
	AllowedUsers  = "allowed_users"
)

type GenKeysMsg struct {
	Amount int `json:"amount"`
}

type GenKeysResponse struct {
	Keys []string `json:"keys"`
}

type KeyStorage struct {
	Key    string
	UsedBy string
	Used   bool
}

type ClaimKeyMsg struct {
	Key string `json:"key"`
}

type ClaimKeyRes struct {
	Success bool `json:"success"`
}

func WriteVerified(ctx context.Context, nk runtime.NakamaModule, userID string) error {
	type verified struct {
	}
	bz, err := json.Marshal(verified{})
	if err != nil {
		return eris.Wrap(err, "")
	}
	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{
		{
			Collection:      AllowedUsers,
			Key:             userID,
			UserID:          signer.AdminAccountID,
			Value:           string(bz),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		},
	})
	return err
}

// IsUserVerified returns true if the user has registered a beta key and false if they have not registered a beta key.
func IsUserVerified(ctx context.Context, nk runtime.NakamaModule, userID string) (verified bool, err error) {
	if !Enabled {
		// When allowlist is disabled, treat all users as if they were on the allowlist
		return true, nil
	}
	objs, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{
			Collection: AllowedUsers,
			Key:        userID,
			UserID:     signer.AdminAccountID,
		},
	})
	if err != nil {
		return false, eris.Wrap(err, "")
	}
	if len(objs) == 0 {
		return false, nil
	}
	return true, nil
}

func readKey(ctx context.Context, nk runtime.NakamaModule, key string) (*KeyStorage, error) {
	objs, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{
			Collection: KeyCollection,
			Key:        key,
			UserID:     signer.AdminAccountID,
		},
	})
	if err != nil {
		return nil, eris.Wrap(err, "error reading storage object for key")
	}
	if len(objs) == 0 {
		return nil, eris.Wrap(nakamaerrors.ErrInvalidBetaKey, "")
	}

	obj := objs[0]
	var ks KeyStorage
	err = json.Unmarshal([]byte(obj.Value), &ks)
	if err != nil {
		return nil, eris.Wrapf(err, "could not unmarshal storage object into %T", ks)
	}
	return &ks, nil
}

func writeKey(ctx context.Context, nk runtime.NakamaModule, ks *KeyStorage) error {
	bz, err := json.Marshal(ks)
	if err != nil {
		return eris.Wrapf(err, "could not marshal KeyStorage object")
	}
	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{
		{
			Collection:      KeyCollection,
			Key:             ks.Key,
			UserID:          signer.AdminAccountID,
			Value:           string(bz),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		},
	})
	if err != nil {
		return eris.Wrapf(err, "could not write KeyObject back to storage")
	}
	return nil
}

func ClaimKey(ctx context.Context, nk runtime.NakamaModule, key, userID string) error {
	ks, err := readKey(ctx, nk, key)
	if err != nil {
		return err
	}
	if ks.Used {
		return eris.Wrapf(nakamaerrors.ErrBetaKeyAlreadyUsed, "user %q was unable to claim %q", userID, key)
	}
	ks.Used = true
	ks.UsedBy = userID

	err = writeKey(ctx, nk, ks)
	if err != nil {
		return err
	}

	return nil
}

func generateRandomBytes(n int) ([]byte, error) {
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return bytes, nil
}

func GenerateBetaKeys(n int) ([]string, error) {
	const bzLen = 16
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		randomBytes, err := generateRandomBytes(bzLen) // 16 bytes for the desired format
		if err != nil {
			return nil, err
		}
		// Format the random bytes as a hyphen-separated string
		key := hex.EncodeToString(randomBytes)
		key = strings.ToUpper(key)
		key = fmt.Sprintf("%s-%s-%s-%s", key[0:4], key[4:8], key[8:12], key[12:16])
		keys = append(keys, key)
	}

	return keys, nil
}
