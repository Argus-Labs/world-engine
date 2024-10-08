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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcode "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
		ctx, span := otel.Tracer("nakama.rpc").Start(ctx, "nakama/claim-persona",
			trace.WithAttributes(
				attribute.String("payload", payload),
			))
		defer span.End()

		ptr := &persona.StorageObj{}
		span.AddEvent("Unmarshalling payload")
		if err := json.Unmarshal([]byte(payload), ptr); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcode.Error, "Failed to unmarshal payload")
			return utils.LogErrorWithMessageAndCode(
				logger,
				err,
				codes.InvalidArgument,
				"unable to unmarshal payload: %v",
				err)
		}

		span.AddEvent("Claiming persona")
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
			span.SetStatus(otelcode.Ok, "successfully claimed persona")
			return utils.MarshalResult(logger, result)
		}

		span.RecordError(err)
		switch {
		case errors.Is(eris.Cause(err), persona.ErrPersonaTagStorageObjNotFound):
			span.SetStatus(otelcode.Error, "Persona tag storage object not found")
			return utils.LogErrorWithMessageAndCode(logger, err, codes.NotFound, "persona tag storage object not found")
		case errors.Is(err, persona.ErrPersonaTagEmpty):
			span.SetStatus(otelcode.Error, "Missing personaTag field")
			return utils.LogErrorWithMessageAndCode(
				logger,
				err,
				codes.InvalidArgument,
				"claim persona tag request must have personaTag field",
			)
		}
		span.SetStatus(otelcode.Error, "Unknown error")
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
		ctx, span := otel.Tracer("nakama.rpc").Start(ctx, "nakama/show-persona")
		defer span.End()

		span.AddEvent("Getting persona from storage")
		result, err := persona.ShowPersona(ctx, nk, txSigner, cardinalAddress)
		if err == nil {
			span.SetStatus(otelcode.Ok, "successfully showed persona")
			return utils.MarshalResult(logger, result)
		}

		span.RecordError(err)
		if eris.Is(eris.Cause(err), persona.ErrPersonaTagStorageObjNotFound) {
			span.SetStatus(otelcode.Error, "Persona tag not found")
			return utils.LogErrorWithMessageAndCode(logger, err, codes.NotFound, "persona tag not found")
		}
		span.SetStatus(otelcode.Error, "Unknown error")
		return utils.LogError(logger, err, codes.FailedPrecondition)
	}
}

func handleGenerateKey(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	ctx, span := otel.Tracer("nakama.rpc").Start(ctx, "generate-beta-keys",
		trace.WithAttributes(
			attribute.String("payload", payload),
		))
	defer span.End()

	var gk allowlist.GenKeysMsg
	span.AddEvent("Unmarshalling payload")
	if err := json.Unmarshal([]byte(payload), &gk); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcode.Error, "Failed to unmarshal payload")
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			codes.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}

	span.AddEvent("Generating beta keys")
	result, err := allowlist.GenerateBetaKeys(ctx, nk, gk)
	if err == nil {
		span.SetStatus(otelcode.Ok, "successfully generated beta keys")
		return utils.MarshalResult(logger, result)
	}

	span.RecordError(err)
	switch {
	case errors.Is(err, allowlist.ErrReadingAmountOfKeys):
		span.SetStatus(otelcode.Error, "Key amount incorrectly formatted")
		return utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "key amount incorrectly formatted")
	case errors.Is(err, allowlist.ErrPermissionDenied):
		span.SetStatus(otelcode.Error, "Non-admin user tried to generate beta keys")
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			codes.PermissionDenied,
			"non-admin user tried to call generate-beta-keys",
		)
	}
	span.SetStatus(otelcode.Error, "Unknown error")
	return utils.LogError(logger, err, codes.FailedPrecondition)
}

func handleClaimKey(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string) (
	string, error,
) {
	ctx, span := otel.Tracer("nakama.rpc").Start(ctx, "claim-key",
		trace.WithAttributes(
			attribute.String("payload", payload),
		))
	defer span.End()

	var ck allowlist.ClaimKeyMsg
	span.AddEvent("Unmarshalling payload")
	if err := json.Unmarshal([]byte(payload), &ck); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcode.Error, "Failed to unmarshal payload")
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			codes.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}

	span.AddEvent("Claiming beta key")
	result, err := allowlist.ClaimKey(ctx, nk, ck)
	if err == nil {
		span.SetStatus(otelcode.Ok, "successfully claimed beta key")
		return utils.MarshalResult(logger, result)
	}

	span.RecordError(err)
	switch {
	case errors.Is(err, allowlist.ErrAlreadyVerified):
		span.SetStatus(otelcode.Error, "User is already verified")
		return utils.LogErrorWithMessageAndCode(logger, err, codes.AlreadyExists, "user has already been verified")
	case errors.Is(err, allowlist.ErrInvalidBetaKey):
		span.SetStatus(otelcode.Error, "Invalid beta key")
		return utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "beta key is invalid")
	case errors.Is(err, allowlist.ErrBetaKeyAlreadyUsed):
		span.SetStatus(otelcode.Error, "Beta key has already been used")
		return utils.LogErrorWithMessageAndCode(logger, err, codes.PermissionDenied, "beta key has already been used")
	}
	span.SetStatus(otelcode.Error, "Unknown error")
	return utils.LogError(logger, err, codes.FailedPrecondition)
}

func handleSaveGame(
	ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string,
) (string, error) {
	ctx, span := otel.Tracer("nakama.rpc").Start(ctx, "nakama/save",
		trace.WithAttributes(
			attribute.String("payload", payload),
		))
	defer span.End()

	var msg SaveGameRequest
	span.AddEvent("Unmarshalling payload")
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcode.Error, "Failed to unmarshal payload")
		return utils.LogErrorWithMessageAndCode(
			logger,
			err,
			codes.InvalidArgument,
			"unable to unmarshal payload: %v",
			err)
	}

	span.AddEvent("Writing save data")
	result, err := writeSave(ctx, nk, msg)
	if err == nil {
		span.SetStatus(otelcode.Ok, "successfully saved game")
		return utils.MarshalResult(logger, result)
	}

	span.RecordError(err)
	span.SetStatus(otelcode.Error, "Unknown error")
	return utils.LogError(logger, err, codes.FailedPrecondition)
}

func handleGetSaveGame(
	ctx context.Context,
	logger runtime.Logger,
	_ *sql.DB,
	nk runtime.NakamaModule,
	_ string,
) (string, error) {
	ctx, span := otel.Tracer("nakama.rpc").Start(ctx, "nakama/get-save")
	defer span.End()

	span.AddEvent("Reading save data")
	result, err := readSave(ctx, nk)
	if err == nil {
		span.SetStatus(otelcode.Ok, "successfully retrieved saved game")
		return utils.MarshalResult(logger, result)
	}

	span.RecordError(err)
	if errors.Is(err, ErrNoSaveFound) {
		span.SetStatus(otelcode.Error, "Failed to read save data")
		return utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "failed to read save data")
	}

	span.SetStatus(otelcode.Error, "Unknown error")
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
		// This request may fail if the Cardinal DB has been wiped since Nakama registered this persona tag.
		// This function will:
		// 1) Make the initial request. If this succeeds, great. We're done.
		// 2) Re-register the persona tag if appropriate (The feature may not be enabled or the error may not look like
		//    a missing signer address failure).
		// 3) Make the request again. If this fails again, there's nothing else we can do.
		logger.Debug("Got request for %q", currEndpoint)
		ctx, span := otel.Tracer("nakama.rpc").Start(ctx, currEndpoint)
		defer span.End()

		// //////////////////////////////
		// Try to send the transaction //
		// //////////////////////////////
		span.AddEvent("Creating first payload")
		resultPayload, err := createPayload(payload, currEndpoint, nk, ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcode.Error, "Failed to create payload")
			return utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "unable to make payload")
		}

		span.AddEvent("Sending first request to Cardinal")
		result, err := makeRequestAndReadResp(ctx, notifier, currEndpoint, resultPayload, cardinalAddress)
		if err == nil {
			span.SetStatus(otelcode.Ok, "successfully called cardinal")
			// The request was successful. Return the result.
			return result, nil
		}

		span.RecordError(err)
		initialResult, initialErr := utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "")

		// ///////////////////////////
		// Re-claim the persona tag //
		// ///////////////////////////
		span.AddEvent("Determining if we should reclaim persona")
		if !autoReClaimPersonaTags || !isResultASignerError(err) {
			span.RecordError(initialErr)
			span.SetStatus(otelcode.Error, "Failed to reclaim persona tag")
			// We're not configured to re-register persona tags, or the returned error doesn't even look like
			// a signer address error. Just return the error.
			return initialResult, initialErr
		}

		// The rest of this function will attempt to re-register the persona tag and then re-try the initial request.
		span.AddEvent("Reclaiming persona")
		txHash, err := persona.ReclaimPersona(ctx, nk, txSigner, cardinalAddress, namespace)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcode.Error, "Failed to re-register persona tag")
			logger.Error("failed to re-register the persona tag: %v", err)
			return initialResult, initialErr
		}

		// The ReclaimPersona request was successful, now we need to wait for a Cardinal tick to be completed.
		// This is a bad practice, but plumbing the events system here for an essentially dev-only behavior seems
		// worse.
		span.AddEvent("Waiting until persona tag has been processed")
		blockUntilPersonaTagTxHasBeenProcessed(logger, eventHub, txHash)

		// /////////////////////////////////
		// Repeat the initial transaction //
		// /////////////////////////////////
		span.AddEvent("Creating second payload")
		resultPayload, err = createPayload(payload, currEndpoint, nk, ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcode.Error, "Failed to create payload")
			return utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "unable to make payload")
		}

		span.AddEvent("Sending second request to Cardinal")
		result, err = makeRequestAndReadResp(ctx, notifier, currEndpoint, resultPayload, cardinalAddress)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcode.Error, "Failed to retry call cardinal")
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
