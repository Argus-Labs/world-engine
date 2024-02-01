package allowlist

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"
	"strings"
)

var (
	EnabledEnvVar = "ENABLE_ALLOWLIST"
	Enabled       = false
	KeyCollection = "allowlist_keys_collection"
	AllowedUsers  = "allowed_users"

	ErrReadingAmountOfKeys  = errors.New("error unmarshalling payload: expected form {\"amount\": <int>}")
	ErrFailedToGenerateKeys = errors.New("error generating beta keys")
	ErrPermissionDenied     = errors.New("permission denied: caller is not admin")

	ErrInvalidBetaKey     = errors.New("invalid beta key")
	ErrNotAllowlisted     = errors.New("this user is not allowlisted")
	ErrBetaKeyAlreadyUsed = errors.New("beta key already used")
	ErrAlreadyVerified    = errors.New("this user is already verified by an existing beta key")
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

func GenerateBetaKeys(ctx context.Context, nk runtime.NakamaModule, msg GenKeysMsg) (res GenKeysResponse, err error) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return res, eris.Wrap(err, "failed to get userID for claim key request")
	}

	if userID != signer.AdminAccountID {
		return res, eris.Wrap(ErrPermissionDenied, "unauthorized: only admin may call generate-beta-keys")
	}

	const bzLen = 16
	keys := make([]string, 0, msg.Amount)
	for i := 0; i < msg.Amount; i++ {
		randomBytes, err := generateRandomBytes(bzLen) // 16 bytes for the desired format
		if err != nil {
			return res, err
		}
		// Format the random bytes as a hyphen-separated string
		key := hex.EncodeToString(randomBytes)
		key = strings.ToUpper(key)
		key = fmt.Sprintf("%s-%s-%s-%s", key[0:4], key[4:8], key[8:12], key[12:16])
		keys = append(keys, key)
	}

	writes := make([]*runtime.StorageWrite, 0, len(keys))
	for _, key := range keys {
		obj := KeyStorage{
			Key:    key,
			UsedBy: "",
			Used:   false,
		}
		bz, err := json.Marshal(obj)
		if err != nil {
			return res, err
		}
		writes = append(writes, &runtime.StorageWrite{
			Collection:      KeyCollection,
			Key:             key,
			UserID:          userID,
			Value:           string(bz),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		})
	}

	_, err = nk.StorageWrite(ctx, writes)
	if err != nil {
		return res, err
	}

	return GenKeysResponse{Keys: keys}, nil
}

type ClaimKeyMsg struct {
	Key string `json:"key"`
}

type ClaimKeyRes struct {
	Success bool `json:"success"`
}

func ClaimKey(ctx context.Context, nk runtime.NakamaModule, msg ClaimKeyMsg) (res ClaimKeyRes, err error) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return res, eris.Wrap(err, "failed to get userID for claim key request")
	}

	verified, err := IsUserVerified(ctx, nk, userID)
	if err != nil {
		return res, eris.Wrap(err, "failed to check if user is validated")
	}
	if verified {
		return res, eris.Wrap(ErrAlreadyVerified, "")
	}

	if msg.Key == "" {
		return res, ErrInvalidBetaKey
	}
	msg.Key = strings.ToUpper(msg.Key)

	ks, err := readKey(ctx, nk, msg.Key)
	if err != nil {
		return res, err
	}
	if ks.Used {
		return res, eris.Wrapf(ErrBetaKeyAlreadyUsed, "user %q was unable to claim %q", userID, msg.Key)
	}
	ks.Used = true
	ks.UsedBy = userID

	err = writeKey(ctx, nk, ks)
	if err != nil {
		return res, err
	}

	err = writeVerified(ctx, nk, userID)
	if err != nil {
		return res, err
	}

	return ClaimKeyRes{true}, nil
}

func writeVerified(ctx context.Context, nk runtime.NakamaModule, userID string) error {
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
		return nil, eris.Wrap(ErrInvalidBetaKey, "")
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

func generateRandomBytes(n int) ([]byte, error) {
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return bytes, nil
}
