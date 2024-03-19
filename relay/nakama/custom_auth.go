package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"

	"pkg.world.dev/world-engine/relay/nakama/siwe"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

const (
	signInWithEthereumType = "siwe"
)

func initCustomAuthentication(initializer runtime.Initializer) error {
	if err := initializer.RegisterBeforeAuthenticateCustom(handleCustomAuthentication); err != nil {
		return eris.Wrap(err, "failed to init siwe authentication")
	}
	return nil
}

func handleCustomAuthentication(
	ctx context.Context,
	logger runtime.Logger,
	_ *sql.DB,
	nk runtime.NakamaModule,
	in *api.AuthenticateCustomRequest) (*api.AuthenticateCustomRequest, error) {

	authType := in.GetAccount().GetVars()["type"]
	// In the future, other authentication methods can be added here (e.g. Twitter)
	if authType == signInWithEthereumType {
		return handleSIWEAuthentication(ctx, logger, nk, in)
	}
	return nil, fmt.Errorf("missing or unknown authentication type: %q", authType)
}

func handleSIWEAuthentication(
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

	// process this request in the siwe package. This single HandleSIWE has the dual purpose of
	// 1) generating a new siwe message when the message and signature are empty and
	// 2) validating a siwe message and signature if they are both present
	// This single method could be split into 2 methods, however testing this handleSIWEAuthentication is tricky
	// in unit tests. Keeping the majority of the business logic in the single HandleSIWE makes it more testable.
	isAuthSuccessful, resp, err := siwe.HandleSIWE(ctx, nk, signerAddress, message, signature)
	if err == nil {
		if isAuthSuccessful {
			// A message and signature was provided, and the signature is valid. The user can now be authenticated.
			return in, nil
		}
		// There is no error, and the user is not authenticated. This means a new SIWE message was generated.
		// Marshal it to JSON and return it with an Unauthorized error.
		bz, jsonErr := json.Marshal(resp)
		if jsonErr != nil {
			_, jsonErr = utils.LogErrorWithMessageAndCode(logger, err, codes.FailedPrecondition, "")
			return nil, jsonErr
		}
		return nil, runtime.NewError(string(bz), int(codes.Unauthenticated))
	}

	switch {
	case errors.Is(err, siwe.ErrMissingSignerAddress):
		_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "id field must be set")
	case errors.Is(err, siwe.ErrMissingSignature):
		fallthrough
	case errors.Is(err, siwe.ErrMissingMessage):
		_, err = utils.LogErrorWithMessageAndCode(logger, err, codes.InvalidArgument, "missing field")
		return nil, err
	}
	_, err = utils.LogError(logger, err, codes.FailedPrecondition)
	return nil, err
}
