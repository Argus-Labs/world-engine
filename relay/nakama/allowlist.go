package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"pkg.world.dev/world-engine/relay/nakama/nakama_errors"
	"pkg.world.dev/world-engine/relay/nakama/utils"
	"strconv"
	"strings"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

var (
	allowlistEnabledEnvVar = "ENABLE_ALLOWLIST"
	allowlistEnabled       = false
	allowlistKeyCollection = "allowlist_keys_collection"
	allowedUsers           = "allowed_users"
)

func initAllowlist(_ runtime.Logger, initializer runtime.Initializer) error {
	enabledStr := os.Getenv(allowlistEnabledEnvVar)
	if enabledStr == "" {
		return nil
	}
	var err error
	allowlistEnabled, err = strconv.ParseBool(enabledStr)
	if err != nil {
		return eris.Wrapf(err, "the ENABLE_ALLOWLIST flag was set, however the variable %q was an invalid ", enabledStr)
	}

	if !allowlistEnabled {
		return nil
	}
	err = initializer.RegisterRpc("generate-beta-keys", allowListRPC)
	if err != nil {
		return eris.Wrap(err, "failed to register rpc")
	}

	err = initializer.RegisterRpc("claim-key", claimKeyRPC)
	if err != nil {
		return eris.Wrap(err, "failed to register rpc")
	}
	return nil
}

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

func allowListRPC(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	id, err := utils.GetUserID(ctx)
	if err != nil {
		return utils.LogErrorFailedPrecondition(logger, err)
	}
	if id != adminAccountID {
		return utils.LogError(
			logger,
			eris.Errorf("unauthorized: only admin may call this RPC"),
			nakama_errors.PermissionDenied,
		)
	}

	var msg GenKeysMsg
	err = json.Unmarshal([]byte(payload), &msg)
	if err != nil {
		return utils.LogError(
			logger,
			eris.Wrap(err, `error unmarshalling payload: expected form {"amount": <int>}`),
			nakama_errors.InvalidArgument)
	}

	keys, err := generateBetaKeys(msg.Amount)
	if err != nil {
		return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "error generating beta keys"))
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
			return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to marshal generated key")
		}
		writes = append(writes, &runtime.StorageWrite{
			Collection:      allowlistKeyCollection,
			Key:             key,
			UserID:          id,
			Value:           string(bz),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		})
	}

	_, err = nk.StorageWrite(ctx, writes)
	if err != nil {
		return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "error writing keys to storage"))
	}

	response, err := json.Marshal(GenKeysResponse{Keys: keys})
	if err != nil {
		return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, ""))
	}
	return string(response), nil
}

type ClaimKeyMsg struct {
	Key string `json:"key"`
}

type ClaimKeyRes struct {
	Success bool `json:"success"`
}

func claimKeyRPC(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return utils.LogErrorWithMessageAndCode(logger, err, nakama_errors.NotFound, "unable to get userID: %v", err)
	}

	if verified, err := isUserVerified(ctx, nk, userID); err != nil {
		return utils.LogErrorMessageFailedPrecondition(logger, err, "failed to check beta key status")
	} else if verified {
		msg := fmt.Sprintf("user %q already verified with a beta key", userID)
		return utils.LogErrorWithMessageAndCode(logger, nakama_errors.ErrAlreadyVerified, nakama_errors.AlreadyExists, msg)
	}

	var ck ClaimKeyMsg
	err = json.Unmarshal([]byte(payload), &ck)
	if err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			nakama_errors.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}
	if ck.Key == "" {
		return utils.LogErrorWithMessageAndCode(
			logger,
			nakama_errors.ErrInvalidBetaKey,
			nakama_errors.InvalidArgument,
			"no key provided in request")
	}
	ck.Key = strings.ToUpper(ck.Key)
	err = claimKey(ctx, nk, ck.Key, userID)
	if err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			nakama_errors.InvalidArgument,
			fmt.Sprintf("unable to claim key: %v", err))
	}
	err = writeVerified(ctx, nk, userID)
	if err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			nakama_errors.NotFound,
			fmt.Sprintf("server could not save user verification entry. please "+
				"try again: %v", err))
	}

	bz, err := json.Marshal(ClaimKeyRes{Success: true})
	if err != nil {
		return utils.LogErrorWithMessageAndCode(logger, err, nakama_errors.NotFound, "unable to marshal response: %v", err)
	}
	return string(bz), nil
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
			Collection:      allowedUsers,
			Key:             userID,
			UserID:          adminAccountID,
			Value:           string(bz),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		},
	})
	return err
}

// isUserVerified returns true if the user has registered a beta key and false if they have not registered a beta key.
func isUserVerified(ctx context.Context, nk runtime.NakamaModule, userID string) (verified bool, err error) {
	if !allowlistEnabled {
		// When allowlist is disabled, treat all users as if they were on the allowlist
		return true, nil
	}
	objs, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{
			Collection: allowedUsers,
			Key:        userID,
			UserID:     adminAccountID,
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
			Collection: allowlistKeyCollection,
			Key:        key,
			UserID:     adminAccountID,
		},
	})
	if err != nil {
		return nil, eris.Wrap(err, "error reading storage object for key")
	}
	if len(objs) == 0 {
		return nil, eris.Wrap(nakama_errors.ErrInvalidBetaKey, "")
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
			Collection:      allowlistKeyCollection,
			Key:             ks.Key,
			UserID:          adminAccountID,
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

func claimKey(ctx context.Context, nk runtime.NakamaModule, key, userID string) error {
	ks, err := readKey(ctx, nk, key)
	if err != nil {
		return err
	}
	if ks.Used {
		return eris.Wrapf(nakama_errors.ErrBetaKeyAlreadyUsed, "user %q was unable to claim %q", userID, key)
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

func generateBetaKeys(n int) ([]string, error) {
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
