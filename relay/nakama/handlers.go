package main

import (
	"context"
	"database/sql"
	"encoding/json"
	errors2 "errors"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"io"
	"net/http"
	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/errors"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"pkg.world.dev/world-engine/relay/nakama/utils"
	"strings"
)

// handleClaimPersona handles a request to Nakama to associate the current user with the persona tag in the payload.
func handleClaimPersona(verifier *persona.Verifier, notifier *receipt.Notifier) nakamaRPCHandler {
	return func(
		ctx context.Context,
		logger runtime.Logger,
		db *sql.DB,
		nk runtime.NakamaModule,
		payload string,
	) (string, error) {
		ptr := &persona.StorageObj{}
		if err := json.Unmarshal([]byte(payload), ptr); err != nil {
			return utils.LogErrorWithMessageAndCode(
				logger,
				err,
				errors.InvalidArgument,
				"unable to unmarshal payload: %v",
				err)
		}

		result, err := persona.ClaimPersona(
			ctx,
			nk,
			verifier,
			notifier,
			ptr,
			globalCardinalAddress,
			globalNamespace,
			&globalPersonaTagAssignment,
		)
		if err == nil {
			return utils.MarshalResult(logger, result)
		}

		switch {
		case errors2.Is(eris.Cause(err), persona.ErrPersonaTagStorageObjNotFound):
			return utils.LogErrorWithMessageAndCode(logger, err, errors.NotFound, "persona tag storage object not found")
		case errors2.Is(err, persona.ErrPersonaTagEmpty):
			return utils.LogErrorWithMessageAndCode(
				logger,
				err,
				errors.InvalidArgument,
				"claim persona tag request must have personaTag field",
			)
		}
		return utils.LogError(logger, err, errors.Internal)
	}
}

func handleShowPersona(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, _ string,
) (string, error) {
	result, err := persona.ShowPersona(ctx, nk, globalCardinalAddress)
	if err == nil {
		return utils.MarshalResult(logger, result)
	}

	if eris.Is(eris.Cause(err), persona.ErrPersonaTagStorageObjNotFound) {
		return utils.LogErrorWithMessageAndCode(logger, err, errors.NotFound, "persona tag not found")
	}
	return utils.LogError(logger, err, errors.Internal)
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
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			errors.PermissionDenied,
			"non-admin user tried to call generate-beta-keys",
		)
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

func handleGetSaveGame(
	ctx context.Context,
	logger runtime.Logger,
	_ *sql.DB,
	nk runtime.NakamaModule,
	_ string,
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

//nolint:gocognit
func handleCardinalRequest(
	currEndpoint string,
	createPayload func(string, string, runtime.NakamaModule, context.Context) (io.Reader, error),
	notifier *receipt.Notifier,
) nakamaRPCHandler {
	return func(
		ctx context.Context,
		logger runtime.Logger,
		db *sql.DB,
		nk runtime.NakamaModule,
		payload string,
	) (string, error) {
		logger.Debug("Got request for %q", currEndpoint)
		var resultPayload io.Reader
		resultPayload, err := createPayload(payload, currEndpoint, nk, ctx)
		if err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to make payload")
		}

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			utils.MakeHTTPURL(currEndpoint, globalCardinalAddress),
			resultPayload,
		)
		req.Header.Set("Content-Type", "application/json")
		if err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "request setup failed for endpoint %q", currEndpoint)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "request failed for endpoint %q", currEndpoint)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return utils.LogErrorMessageFailedPrecondition(
					logger,
					eris.Wrap(err, "failed to read response body"),
					"bad status code: %s: %s", resp.Status, body,
				)
			}
			return utils.LogErrorMessageFailedPrecondition(
				logger,
				eris.Errorf("bad status code %d", resp.StatusCode),
				"bad status code: %s: %s", resp.Status, body,
			)
		}
		bz, err := io.ReadAll(resp.Body)
		if err != nil {
			return utils.LogErrorMessageFailedPrecondition(logger, err, "can't read body")
		}
		if strings.HasPrefix(currEndpoint, TransactionEndpointPrefix) {
			var asTx persona.TxResponse

			if err = json.Unmarshal(bz, &asTx); err != nil {
				return utils.LogErrorMessageFailedPrecondition(logger, err, "can't decode body as tx response")
			}
			userID, err := utils.GetUserID(ctx)
			if err != nil {
				return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to get user id")
			}
			notifier.AddTxHashToPendingNotifications(asTx.TxHash, userID)
		}

		return string(bz), nil
	}
}

// nakamaRPCHandler is the signature required for handlers that are passed to Nakama's RegisterRpc method.
// This type is defined just to make the function below a little more readable.
type nakamaRPCHandler func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule,
	payload string) (string, error)
