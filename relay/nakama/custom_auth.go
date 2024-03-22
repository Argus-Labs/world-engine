package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

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

var (
	ErrBadCustomAuthType = errors.New("bad custom auth type")
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
		return handleSIWE(ctx, logger, nk, in)
	}
	return nil, ErrBadCustomAuthType
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

	if signerAddress == "" {
		_, err := utils.LogErrorWithMessageAndCode(
			logger, siwe.ErrMissingSignerAddress, codes.InvalidArgument, "id field must be set")
		return nil, err
	}
	if signature == "" && message != "" {
		_, err := utils.LogErrorWithMessageAndCode(
			logger, siwe.ErrMissingSignature, codes.InvalidArgument, "missing field")
		return nil, err
	}
	if signature != "" && message == "" {
		_, err := utils.LogErrorWithMessageAndCode(
			logger, siwe.ErrMissingMessage, codes.InvalidArgument, "missing field")
		return nil, err
	}

	if signature != "" && message != "" {
		// The user has provided a signature and a message. Attempt to authenticate the user.
		if err := siwe.ValidateSignature(ctx, nk, signerAddress, message, signature); err != nil {
			_, err = utils.LogErrorWithMessageAndCode(
				logger, siwe.ErrMissingMessage, codes.Unauthenticated, "authentication failed")
			return nil, err
		}
		return in, nil
	}

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
	return nil, runtime.NewError(string(bz), int(codes.Unauthenticated))
}
