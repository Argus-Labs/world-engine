package auth

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"

	"pkg.world.dev/world-engine/relay/nakama/siwe"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

func validateSIWE(signer, message, signature string) (isValidationRequest bool, err error) {
	var errs []error
	if signer == "" {
		errs = append(errs, eris.Wrap(siwe.ErrMissingSignerAddress, "id field must be set"))
	}
	if signature == "" && message != "" {
		errs = append(errs, eris.Wrap(siwe.ErrMissingSignature, "signature field must be set"))
	}
	if signature != "" && message == "" {
		errs = append(errs, eris.Wrap(siwe.ErrMissingMessage, "message field must be set"))
	}
	if len(errs) > 0 {
		return false, errors.Join(errs...)
	}
	if signature != "" && message != "" {
		isValidationRequest = true
	}
	return isValidationRequest, nil
}

func processSIWE(
	ctx context.Context,
	logger runtime.Logger,
	nk runtime.NakamaModule,
	signerAddress, signature, message string,
	span trace.Span,
) error {
	span.AddEvent("Validating SIWE request")
	isValidationRequest, err := validateSIWE(signerAddress, message, signature)
	if err != nil {
		_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "invalid vars")
		return err
	}

	if !isValidationRequest {
		span.AddEvent("Generating SIWE message to sign")
		// The signature and message is empty. Generate a new SIWE message for the user.
		resp, err := siwe.GenerateNewSIWEMessage(signerAddress)
		if err != nil {
			_, err = utils.LogError(logger, err, codes.FailedPrecondition)
			return err
		}

		span.AddEvent("Marshalling SIWE message into JSON")
		bz, err := json.Marshal(resp)
		if err != nil {
			_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "")
			return err
		}
		// An error is being returned here, but the error contains the text of the
		// SIWE Message that needs to be signed.
		return runtime.NewError(string(bz), int(codes.Unauthenticated))
	}

	span.AddEvent("Validating SIWE signature")
	// The user has provided a signature and a message. Attempt to authenticate the user.
	if err := siwe.ValidateSignature(ctx, nk, signerAddress, message, signature); err != nil {
		_, err = utils.LogErrorWithMessageAndCode(
			logger, err, codes.Unauthenticated, "authentication failed")
		return err
	}

	// The user has successfully been authenticated
	return nil
}

func authWithSIWE(
	ctx context.Context,
	logger runtime.Logger,
	nk runtime.NakamaModule,
	in *api.AuthenticateCustomRequest,
	span trace.Span,
) (*api.AuthenticateCustomRequest, error) {
	signerAddress := in.GetAccount().GetId()
	signature := in.GetAccount().GetVars()["signature"]
	message := in.GetAccount().GetVars()["message"]

	err := processSIWE(ctx, logger, nk, signerAddress, signature, message, span)
	if err != nil {
		return nil, err
	}

	// The user has successfully been authenticated
	return in, err
}

func linkWithSIWE(
	ctx context.Context,
	logger runtime.Logger,
	nk runtime.NakamaModule,
	in *api.AccountCustom,
	span trace.Span,
) (*api.AccountCustom, error) {
	signerAddress := in.GetId()
	signature := in.GetVars()["signature"]
	message := in.GetVars()["message"]

	err := processSIWE(ctx, logger, nk, signerAddress, signature, message, span)
	if err != nil {
		return nil, err
	}

	// The user has successfully been authenticated
	return in, err
}
