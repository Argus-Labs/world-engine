package main

import (
	"context"
	"database/sql"
	"encoding/json"
	errors2 "errors"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/errors"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

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
				allowlist.ErrNotAllowlisted,
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

func handleGenerateKey(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	var gk allowlist.GenKeysMsg
	if err := json.Unmarshal([]byte(payload), &gk); err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			errors.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}

	result, err := allowlist.GenerateBetaKeys(ctx, nk, gk)
	if err == nil {
		return utils.MarshalResult(logger, result)
	}

	switch {
	case errors2.Is(err, allowlist.ErrReadingAmountOfKeys):
		return utils.LogErrorWithMessageAndCode(logger, err, errors.InvalidArgument, "key amount incorrectly formatted")
	case errors2.Is(err, allowlist.ErrPermissionDenied):
		return utils.LogErrorWithMessageAndCode(logger, err, errors.PermissionDenied, "non-admin user tried to call generate-beta-keys")
	}
	return utils.LogError(logger, err, errors.Internal)
}

func handleClaimKey(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	var ck allowlist.ClaimKeyMsg
	if err := json.Unmarshal([]byte(payload), &ck); err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			errors.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}

	result, err := allowlist.ClaimKey(ctx, nk, ck)
	if err == nil {
		return utils.MarshalResult(logger, result)
	}

	switch {
	case errors2.Is(err, allowlist.ErrAlreadyVerified):
		return utils.LogErrorWithMessageAndCode(logger, err, errors.AlreadyExists, "user has already been verified")
	case errors2.Is(err, allowlist.ErrInvalidBetaKey):
		return utils.LogErrorWithMessageAndCode(logger, err, errors.InvalidArgument, "beta key is invalid")
	case errors2.Is(err, allowlist.ErrBetaKeyAlreadyUsed):
		return utils.LogErrorWithMessageAndCode(logger, err, errors.PermissionDenied, "beta key has already been used")
	}
	return utils.LogError(logger, err, errors.Internal)
}

func handleSaveGame(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string,
) (string, error) {
	var msg SaveGameRequest
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			errors.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}

	result, err := writeSave(ctx, nk, msg)
	if err == nil {
		return utils.MarshalResult(logger, result)
	}

	return utils.LogError(logger, err, errors.Internal)
}

func handleGetSaveGame(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, _ string,
) (string, error) {

	result, err := readSave(ctx, nk)
	if err == nil {
		return utils.MarshalResult(logger, result)
	}

	if errors2.Is(err, ErrNoSaveFound) {
		return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "failed to read save data"))
	}

	return utils.LogError(logger, err, errors.Internal)
}

// nakamaRPCHandler is the signature required for handlers that are passed to Nakama's RegisterRpc method.
// This type is defined just to make the function below a little more readable.
type nakamaRPCHandler func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule,
	payload string) (string, error)
