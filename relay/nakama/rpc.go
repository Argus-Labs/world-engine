package main

import (
	"context"
	"database/sql"
	"encoding/json"
	errors2 "errors"
	"fmt"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"os"
	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/errors"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"
	"strconv"
	"strings"
)

// initPersonaEndpoints sets up the nakame RPC endpoints that are used to claim a persona tag and display a persona tag.
func initPersonaTagEndpoints(
	_ runtime.Logger,
	initializer runtime.Initializer,
	ptv *persona.Verifier,
	notifier *receipt.Notifier) error {
	if err := initializer.RegisterRpc("nakama/claim-persona", handleClaimPersona(ptv, notifier)); err != nil {
		return eris.Wrap(err, "")
	}
	return eris.Wrap(initializer.RegisterRpc("nakama/show-persona", handleShowPersona), "")
}

// handleClaimPersona handles a request to Nakama to associate the current user with the persona tag in the payload.
//
//nolint:gocognit
func handleClaimPersona(ptv *persona.Verifier, notifier *receipt.Notifier) nakamaRPCHandler {
	return func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (
		string, error) {
		userID, err := utils.GetUserID(ctx)
		if err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to get userID")
		}

		// check if the user is verified. this requires them to input a valid beta key.
		if verified, err := allowlist.IsUserVerified(ctx, nk, userID); err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to claim persona tag")
		} else if !verified {
			return utils.LogDebugWithMessageAndCode(
				logger,
				errors.ErrNotAllowlisted,
				errors.AlreadyExists,
				"unable to claim persona tag")
		}

		ptr := &persona.StorageObj{}
		if err := json.Unmarshal([]byte(payload), ptr); err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, eris.Wrap(err, ""), "unable to marshal payload")
		}
		if ptr.PersonaTag == "" {
			return utils.LogErrorWithMessageAndCode(
				logger,
				eris.New("personaTag field was empty"),
				errors.InvalidArgument,
				"personaTag field must not be empty",
			)
		}

		tag, err := persona.LoadPersonaTagStorageObj(ctx, nk)
		if err != nil {
			if !errors2.Is(err, errors.ErrPersonaTagStorageObjNotFound) {
				return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to get persona tag storage object")
			}
		} else {
			switch tag.Status {
			case persona.StatusPending:
				return utils.LogDebugWithMessageAndCode(
					logger,
					eris.Errorf("persona tag %q is pending for this account", tag.PersonaTag),
					errors.AlreadyExists,
					"persona tag %q is pending", tag.PersonaTag,
				)
			case persona.StatusAccepted:
				return utils.LogErrorWithMessageAndCode(
					logger,
					eris.Errorf("persona tag %q already associated with this account", tag.PersonaTag),
					errors.AlreadyExists,
					"persona tag %q already associated with this account",
					tag.PersonaTag)
			case persona.StatusRejected:
				// if the tag was rejected, don't do anything. let the user try to claim another tag.
			}
		}

		txHash, tick, err := persona.CardinalCreatePersona(ctx, nk, ptr.PersonaTag, globalCardinalAddress, globalNamespace)
		if err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to make create persona request to cardinal")
		}
		notifier.AddTxHashToPendingNotifications(txHash, userID)

		ptr.Status = persona.StatusPending
		if err = ptr.SavePersonaTagStorageObj(ctx, nk); err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to set persona tag storage object")
		}

		// Try to actually assign this personaTag->UserID in the sync map. If this succeeds, Nakama is OK with this
		// user having the persona tag.
		if ok := setPersonaTagAssignment(ptr.PersonaTag, userID); !ok {
			ptr.Status = persona.StatusRejected
			if err = ptr.SavePersonaTagStorageObj(ctx, nk); err != nil {
				return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to set persona tag storage object")
			}
			return utils.LogErrorWithMessageAndCode(
				logger,
				eris.Errorf("persona tag %q is not available", ptr.PersonaTag),
				errors.AlreadyExists,
				"persona tag %q is not available",
				ptr.PersonaTag)
		}

		ptr.Tick = tick
		ptr.TxHash = txHash
		if err = ptr.SavePersonaTagStorageObj(ctx, nk); err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to save persona tag storage object")
		}
		ptv.AddPendingPersonaTag(userID, ptr.TxHash)
		res, err := ptr.ToJSON()
		if err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to marshal response")
		}
		return res, nil
	}
}

func handleShowPersona(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, _ string,
) (string, error) {
	ptr, err := persona.LoadPersonaTagStorageObj(ctx, nk)
	if err != nil {
		if eris.Is(eris.Cause(err), errors.ErrPersonaTagStorageObjNotFound) {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "no persona tag found")
		}
		return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to get persona tag storage object")
	}
	ptr, err = ptr.AttemptToUpdatePending(ctx, nk, globalCardinalAddress)
	if err != nil {
		return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to update pending state")
	}
	res, err := ptr.ToJSON()
	if err != nil {
		return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to marshal response")
	}
	return res, nil
}

func InitAllowlist(_ runtime.Logger, initializer runtime.Initializer) error {
	enabledStr := os.Getenv(allowlist.EnabledEnvVar)
	if enabledStr == "" {
		return nil
	}
	var err error
	allowlist.Enabled, err = strconv.ParseBool(enabledStr)
	if err != nil {
		return eris.Wrapf(err, "the ENABLE_ALLOWLIST flag was set, however the variable %q was an invalid ", enabledStr)
	}

	if !allowlist.Enabled {
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

func allowListRPC(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	id, err := utils.GetUserID(ctx)
	if err != nil {
		return utils.LogErrorFailedPrecondition(logger, err)
	}
	if id != signer.AdminAccountID {
		return utils.LogError(
			logger,
			eris.Errorf("unauthorized: only admin may call this RPC"),
			errors.PermissionDenied,
		)
	}

	var msg allowlist.GenKeysMsg
	err = json.Unmarshal([]byte(payload), &msg)
	if err != nil {
		return utils.LogError(
			logger,
			eris.Wrap(err, `error unmarshalling payload: expected form {"amount": <int>}`),
			errors.InvalidArgument)
	}

	keys, err := allowlist.GenerateBetaKeys(msg.Amount)
	if err != nil {
		return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "error generating beta keys"))
	}

	writes := make([]*runtime.StorageWrite, 0, len(keys))
	for _, key := range keys {
		obj := allowlist.KeyStorage{
			Key:    key,
			UsedBy: "",
			Used:   false,
		}
		bz, err := json.Marshal(obj)
		if err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to marshal generated key")
		}
		writes = append(writes, &runtime.StorageWrite{
			Collection:      allowlist.KeyCollection,
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

	response, err := json.Marshal(allowlist.GenKeysResponse{Keys: keys})
	if err != nil {
		return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, ""))
	}
	return string(response), nil
}

func claimKeyRPC(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return utils.LogErrorWithMessageAndCode(logger, err, errors.NotFound, "unable to get userID: %v", err)
	}

	if verified, err := allowlist.IsUserVerified(ctx, nk, userID); err != nil {
		return utils.LogErrorMessageFailedPrecondition(logger, err, "failed to check beta key status")
	} else if verified {
		msg := fmt.Sprintf("user %q already verified with a beta key", userID)
		return utils.LogErrorWithMessageAndCode(logger, errors.ErrAlreadyVerified, errors.AlreadyExists, msg)
	}

	var ck allowlist.ClaimKeyMsg
	err = json.Unmarshal([]byte(payload), &ck)
	if err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			errors.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}
	if ck.Key == "" {
		return utils.LogErrorWithMessageAndCode(
			logger,
			errors.ErrInvalidBetaKey,
			errors.InvalidArgument,
			"no key provided in request")
	}
	ck.Key = strings.ToUpper(ck.Key)
	err = allowlist.ClaimKey(ctx, nk, ck.Key, userID)
	if err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			errors.InvalidArgument,
			fmt.Sprintf("unable to claim key: %v", err))
	}
	err = allowlist.WriteVerified(ctx, nk, userID)
	if err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			errors.NotFound,
			fmt.Sprintf("server could not save user verification entry. please "+
				"try again: %v", err))
	}

	bz, err := json.Marshal(allowlist.ClaimKeyRes{Success: true})
	if err != nil {
		return utils.LogErrorWithMessageAndCode(logger, err, errors.NotFound, "unable to marshal response: %v", err)
	}
	return string(bz), nil
}

func initSaveFileStorage(_ runtime.Logger, initializer runtime.Initializer) error {
	err := initializer.RegisterRpc(
		"nakama/save",
		handleSaveGame,
	)
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}

func handleSaveGame(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string,
) (string, error) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return utils.LogErrorMessageFailedPrecondition(logger, eris.Wrap(err, ""), "failed to get user ID")
	}

	var msg SaveGameRequest
	err = json.Unmarshal([]byte(payload), &msg)
	if err != nil {
		return utils.LogError(
			logger,
			eris.Wrap(err, `error unmarshalling payload: expected form {"data": <string>}`),
			errors.InvalidArgument)
	}
	// do not allow empty requests
	if msg.Data == "" {
		return utils.LogErrorFailedPrecondition(
			logger,
			eris.New("data cannot be empty"),
		)
	}

	err = writeSave(ctx, userID, payload, nk)
	if err != nil {
		return utils.LogErrorFailedPrecondition(
			logger,
			eris.Wrap(err, "failed to write game save to storage"),
		)
	}

	response, err := json.Marshal(SaveGameResponse{Success: true})
	if err != nil {
		return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "failed to marshal response"))
	}

	return string(response), nil
}
