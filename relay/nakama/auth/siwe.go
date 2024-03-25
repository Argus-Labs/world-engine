package auth

import (
	"context"
	"encoding/json"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"

	"pkg.world.dev/world-engine/relay/nakama/siwe"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

func validateSIWE(signer, message, signature string) (isValidationRequest bool, err error) {
	if signer == "" {
		return false, eris.Wrap(siwe.ErrMissingSignerAddress, "id field must be set")
	}
	if signature == "" && message != "" {
		return false, eris.Wrap(siwe.ErrMissingSignature, "signature field must be set")
	}
	if signature != "" && message == "" {
		return false, eris.Wrap(siwe.ErrMissingMessage, "signature field must be set")
	}
	if signature != "" && message != "" {
		isValidationRequest = true
	}
	return isValidationRequest, nil
}

func handleSIWE(
	ctx context.Context,
	logger runtime.Logger,
	nk runtime.NakamaModule,
	in *api.AuthenticateCustomRequest,
) (
	*api.AuthenticateCustomRequest, error,
) {
	signerAddress := in.GetAccount().GetId()
	signature := in.GetAccount().GetVars()["signature"]
	message := in.GetAccount().GetVars()["message"]

	isValidationRequest, err := validateSIWE(signerAddress, message, signature)
	if err != nil {
		_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "invalid vars")
		return nil, err
	}

	if !isValidationRequest {
		// The signature and message is empty. Generate a new SIWE message for the user.
		resp, err := siwe.GenerateNewSIWEMessage(signerAddress)
		if err != nil {
			_, err = utils.LogError(logger, err, codes.FailedPrecondition)
			return nil, err
		}

		bz, err := json.Marshal(resp)
		if err != nil {
			_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "")
			return nil, err
		}
		// An error is being returned here, but the error contains the text of the
		// SIWE Message that needs to be signed.
		return nil, runtime.NewError(string(bz), int(codes.Unauthenticated))
	}

	// The user has provided a signature and a message. Attempt to authenticate the user.
	if err := siwe.ValidateSignature(ctx, nk, signerAddress, message, signature); err != nil {
		_, err = utils.LogErrorWithMessageAndCode(
			logger, siwe.ErrMissingMessage, codes.Unauthenticated, "authentication failed")
		return nil, err
	}
	// The user has successfully been authenticated
	return in, nil
}
