package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/heroiclabs/nakama-common/runtime"
	"os"
	"strconv"
)

var (
	allowlistEnabledEnvVar = "ENABLE_ALLOWLIST"
	allowlistEnabled       = false
	allowlistKeyCollection = "allowlist_keys_collection"
	allowedUsers           = "allowed_users"
)

func initAllowlist(logger runtime.Logger, initializer runtime.Initializer) error {
	enabledStr := os.Getenv(allowlistEnabledEnvVar)
	if enabledStr == "" {
		return nil
	} else {
		var err error
		allowlistEnabled, err = strconv.ParseBool(enabledStr)
		if err != nil {
			return err
		}
	}
	if !allowlistEnabled {
		return nil
	}
	err := initializer.RegisterRpc("generate-beta-keys", allowListRPC)
	if err != nil {
		return err
	}

	err = initializer.RegisterRpc("claim-key", claimKeyRPC)
	if err != nil {
		return err
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
	//UsedOn time.Time
	Used bool
}

func allowListRPC(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	id, err := getUserID(ctx)
	if id != adminAccountID {
		return "", fmt.Errorf("unauthorized: only admin may call this RPC")
	}

	var msg GenKeysMsg
	err = json.Unmarshal([]byte(payload), &msg)
	if err != nil {
		return "", fmt.Errorf(`error unmarshaling payload: expected form {"amount": <int>}: %w`, err)
	}

	keys, err := generateBetaKeys(msg.Amount)
	if err != nil {
		return "", fmt.Errorf("error generating beta keys: %w", err)
	}

	if err != nil {
		return "", err
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
			return "", err
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
		return "", fmt.Errorf("error writing keys to storage: %w", err)
	}

	response, err := json.Marshal(GenKeysResponse{Keys: keys})
	if err != nil {
		return "", err
	}
	return string(response), nil
}

func claimKeyRPC(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	type ClaimKeyMsg struct {
		Key string `json:"key"`
	}
	userID, err := getUserID(ctx)
	if err != nil {
		return "", err
	}
	var ck ClaimKeyMsg
	err = json.Unmarshal([]byte(payload), &ck)
	if err != nil {
		return "", err
	}
	err = claimKey(ctx, nk, ck.Key, userID)
	if err != nil {
		return "", err
	}
	err = writeVerified(ctx, nk, userID)
	if err != nil {
		return "", err
	}

	type ClaimKeyRes struct {
		Success bool `json:"success"`
	}
	bz, err := json.Marshal(ClaimKeyRes{Success: true})
	if err != nil {
		return "", err
	}
	return string(bz), nil
}

func writeVerified(ctx context.Context, nk runtime.NakamaModule, userID string) error {
	type verified struct {
	}
	bz, err := json.Marshal(verified{})
	if err != nil {
		return err
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

func checkVerified(ctx context.Context, nk runtime.NakamaModule, userID string) error {
	if !allowlistEnabled {
		return nil
	}
	objs, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{
			Collection: allowedUsers,
			Key:        userID,
			UserID:     adminAccountID,
		},
	})
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		return fmt.Errorf("this user is not allowlisted")
	}
	return nil
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
		return nil, fmt.Errorf("error reading storage object for key: %w", err)
	}
	obj := objs[0]
	var ks KeyStorage
	err = json.Unmarshal([]byte(obj.Value), &ks)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal storage object into KeyStorage: %w", err)
	}
	return &ks, nil
}

func writeKey(ctx context.Context, nk runtime.NakamaModule, ks *KeyStorage) error {
	bz, err := json.Marshal(ks)
	if err != nil {
		return fmt.Errorf("could not marshal KeyStorage object: %w", err)
	}
	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{
		{
			Collection:      allowlistKeyCollection,
			Key:             string(bz),
			UserID:          adminAccountID,
			Value:           string(bz),
			Version:         "",
			PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
			PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
		},
	})
	if err != nil {
		return fmt.Errorf("could not write KeyObject back to storage: %w", err)
	}
	return nil
}

func claimKey(ctx context.Context, nk runtime.NakamaModule, key, userID string) error {
	ks, err := readKey(ctx, nk, key)
	if err != nil {
		return err
	}
	if ks.Used {
		return fmt.Errorf("key already used")
	}
	ks.Used = true
	//ks.UsedOn = time.Now().UTC()
	ks.UsedBy = userID

	err = writeKey(ctx, nk, ks)
	if err != nil {
		return err
	}

	return nil
}
