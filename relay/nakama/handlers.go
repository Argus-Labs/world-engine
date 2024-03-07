package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"

	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

// handleClaimPersona handles a request to Nakama to associate the current user with the persona tag in the payload.
func handleClaimPersona(
	verifier *persona.Verifier,
	notifier *receipt.Notifier,
	txSigner signer.Signer,
	cardinalAddress string,
	globalNamespace string,
	globalPersonaAssignment *sync.Map,
) nakamaRPCHandler {
	return func(
		ctx context.Context,
		logger runtime.Logger,
		_ *sql.DB,
		nk runtime.NakamaModule,
		payload string,
	) (string, error) {
		ptr := &persona.StorageObj{}
		if err := json.Unmarshal([]byte(payload), ptr); err != nil {
			return utils.LogErrorWithMessageAndCode(
				logger,
				err,
				codes.InvalidArgument,
				"unable to unmarshal payload: %v",
				err)
		}

		result, err := persona.ClaimPersona(
			ctx,
			nk,
			verifier,
			notifier,
			ptr,
			txSigner,
			cardinalAddress,
			globalNamespace,
			globalPersonaAssignment,
		)
		if err == nil {
			return utils.MarshalResult(logger, result)
		}

		switch {
		case errors.Is(eris.Cause(err), persona.ErrPersonaTagStorageObjNotFound):
			return utils.LogErrorWithMessageAndCode(logger, err, codes.NotFound, "persona tag storage object not found")
		case errors.Is(err, persona.ErrPersonaTagEmpty):
			return utils.LogErrorWithMessageAndCode(
				logger,
				err,
				codes.InvalidArgument,
				"claim persona tag request must have personaTag field",
			)
		}
		return utils.LogError(logger, err, codes.FailedPrecondition)
	}
}

func handleShowPersona(txSigner signer.Signer, cardinalAddress string) nakamaRPCHandler {
	return func(ctx context.Context,
		logger runtime.Logger,
		_ *sql.DB,
		nk runtime.NakamaModule,
		_ string,
	) (string, error) {
		result, err := persona.ShowPersona(ctx, nk, txSigner, cardinalAddress)
		if err == nil {
			return utils.MarshalResult(logger, result)
		}

		if eris.Is(eris.Cause(err), persona.ErrPersonaTagStorageObjNotFound) {
			return utils.LogErrorWithMessageAndCode(logger, err, codes.NotFound, "persona tag not found")
		}
		return utils.LogError(logger, err, codes.FailedPrecondition)
	}
}

func handleGenerateKey(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	var gk allowlist.GenKeysMsg
	if err := json.Unmarshal([]byte(payload), &gk); err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			codes.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}

	result, err := allowlist.GenerateBetaKeys(ctx, nk, gk)
	if err == nil {
		return utils.MarshalResult(logger, result)
	}

	switch {
	case errors.Is(err, allowlist.ErrReadingAmountOfKeys):
		return utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "key amount incorrectly formatted")
	case errors.Is(err, allowlist.ErrPermissionDenied):
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			codes.PermissionDenied,
			"non-admin user tried to call generate-beta-keys",
		)
	}
	return utils.LogError(logger, err, codes.FailedPrecondition)
}

func handleClaimKey(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	var ck allowlist.ClaimKeyMsg
	if err := json.Unmarshal([]byte(payload), &ck); err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			codes.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}

	result, err := allowlist.ClaimKey(ctx, nk, ck)
	if err == nil {
		return utils.MarshalResult(logger, result)
	}

	switch {
	case errors.Is(err, allowlist.ErrAlreadyVerified):
		return utils.LogErrorWithMessageAndCode(logger, err, codes.AlreadyExists, "user has already been verified")
	case errors.Is(err, allowlist.ErrInvalidBetaKey):
		return utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "beta key is invalid")
	case errors.Is(err, allowlist.ErrBetaKeyAlreadyUsed):
		return utils.LogErrorWithMessageAndCode(logger, err, codes.PermissionDenied, "beta key has already been used")
	}
	return utils.LogError(logger, err, codes.FailedPrecondition)
}

func handleSaveGame(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string,
) (string, error) {
	var msg SaveGameRequest
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			codes.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}

	result, err := writeSave(ctx, nk, msg)
	if err == nil {
		return utils.MarshalResult(logger, result)
	}

	return utils.LogError(logger, err, codes.FailedPrecondition)
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

	if errors.Is(err, ErrNoSaveFound) {
		return utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "failed to read save data")
	}

	return utils.LogError(logger, err, codes.FailedPrecondition)
}

func handleCardinalRequest(
	currEndpoint string,
	createPayload func(string, string, runtime.NakamaModule, context.Context) (io.Reader, error),
	notifier *receipt.Notifier,
	cardinalAddress string,
) nakamaRPCHandler {
	return func(
		ctx context.Context,
		logger runtime.Logger,
		_ *sql.DB,
		nk runtime.NakamaModule,
		payload string,
	) (string, error) {
		logger.Debug("Got request for %q", currEndpoint)
		var resultPayload io.Reader
		resultPayload, err := createPayload(payload, currEndpoint, nk, ctx)
		if err != nil {
			return utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "unable to make payload")
		}

		result, err := makeRequestAndReadResp(ctx, notifier, currEndpoint, resultPayload, cardinalAddress)
		if err != nil {
			return utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "")
		}

		return result, nil
	}
}

// nakamaRPCHandler is the signature required for handlers that are passed to Nakama's RegisterRpc method.
// This type is defined just to make the function below a little more readable.
type nakamaRPCHandler func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule,
	payload string) (string, error)
