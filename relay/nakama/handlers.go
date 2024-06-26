package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"

	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/events"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

// nakamaRPCHandler is the signature required for handlers that are passed to Nakama's RegisterRpc method.
// This type is defined just to make the function below a little more readable.
type nakamaRPCHandler func(
	ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule,
	payload string,
) (string, error)

// handleClaimPersona handles a request to Nakama to associate the current user with the persona tag in the payload.
func handleClaimPersona(
	verifier *persona.Verifier,
	notifier *events.Notifier,
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
	return func(
		ctx context.Context,
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

func handleSaveGame(
	ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string,
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
	notifier *events.Notifier,
	eventHub *events.EventHub,
	cardinalAddress string,
	namespace string,
	txSigner signer.Signer,
	autoReClaimPersonaTags bool,
) nakamaRPCHandler {
	return func(
		ctx context.Context,
		logger runtime.Logger,
		_ *sql.DB,
		nk runtime.NakamaModule,
		payload string,
	) (string, error) {
		logger.Debug("Got request for %q", currEndpoint)
		// This request may fail if the Cardinal DB has been wiped since Nakama registered this persona tag.
		// This function will:
		// 1) Make the initial request. If this succeeds, great. We're done.
		// 2) Re-register the persona tag if appropriate (The feature may not be enabled or the error may not look like
		//    a missing signer address failure).
		// 3) Make the request again. If this fails again, there's nothing else we can do.

		// //////////////////////////////
		// Try to send the transaction //
		// //////////////////////////////
		resultPayload, err := createPayload(payload, currEndpoint, nk, ctx)
		if err != nil {
			return utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "unable to make payload")
		}
		result, err := makeRequestAndReadResp(ctx, notifier, currEndpoint, resultPayload, cardinalAddress)
		if err == nil {
			// The request was successful. Return the result.
			return result, nil
		}
		initialResult, initialErr := utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "")

		// ///////////////////////////
		// Re-claim the persona tag //
		// ///////////////////////////
		if !autoReClaimPersonaTags || !isResultASignerError(err) {
			// We're not configured to re-register persona tags, or the returned error doesn't even look like
			// a signer address error. Just return the error.
			return initialResult, initialErr
		}

		// The rest of this function will attempt to re-register the persona tag and then re-try the initial request.
		txHash, err := persona.ReclaimPersona(ctx, nk, txSigner, cardinalAddress, namespace)
		if err != nil {
			logger.Error("failed to re-register the persona tag: %v", err)
			return initialResult, initialErr
		}

		// The ReclaimPersona request was successful, now we need to wait for a Cardinal tick to be completed.
		// This is a bad practice, but plumbing the events system here for an essentially dev-only behavior seems
		// worse.
		blockUntilPersonaTagTxHasBeenProcessed(logger, eventHub, txHash)

		// /////////////////////////////////
		// Repeat the initial transaction //
		// /////////////////////////////////
		resultPayload, err = createPayload(payload, currEndpoint, nk, ctx)
		if err != nil {
			return utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "unable to make payload")
		}
		result, err = makeRequestAndReadResp(ctx, notifier, currEndpoint, resultPayload, cardinalAddress)
		if err != nil {
			return utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "")
		}
		return result, nil
	}
}

func blockUntilPersonaTagTxHasBeenProcessed(logger runtime.Logger, eventHub *events.EventHub, txHash string) {
	ch := eventHub.SubscribeToReceipts(txHash)
	defer func() {
		eventHub.Unsubscribe(txHash)
	}()
	done := false
	timeout := time.After(time.Second)
	for !done {
		select {
		case receipts := <-ch:
			for _, receipt := range receipts {
				if receipt.TxHash == txHash {
					// We just care that the person tag re-claim tx was processed. Whether it was successful or not
					// will become apparent when the initial tx is resent.
					logger.Info("result of persona tag re-claim: %v", receipt.Result)
					done = true
					break
				}
			}
		case <-timeout:
			logger.Info("timeout while waiting for persona tag to be re-claimed")
			done = true
		}
	}
}

func isResultASignerError(err error) bool {
	return strings.Contains(err.Error(), "could not get signer for persona")
}
