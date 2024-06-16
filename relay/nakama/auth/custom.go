package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

const (
	signInWithEthereumType = "siwe"
)

var (
	ErrBadCustomAuthType = errors.New("bad custom auth type")
)

func InitCustomAuthentication(initializer runtime.Initializer) error {
	if err := initializer.RegisterBeforeAuthenticateCustom(handleCustomAuthentication); err != nil {
		return eris.Wrap(err, "failed to init custom authentication")
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
		return authWithSIWE(ctx, logger, nk, in)
	}
	return nil, ErrBadCustomAuthType
}

func InitCustomLink(initializer runtime.Initializer) error {
	if err := initializer.RegisterBeforeLinkCustom(handleCustomLink); err != nil {
		return eris.Wrap(err, "failed to init custom link")
	}
	return nil
}

func handleCustomLink(
	ctx context.Context,
	logger runtime.Logger,
	_ *sql.DB,
	nk runtime.NakamaModule,
	in *api.AccountCustom) (*api.AccountCustom, error) {
	authType := in.GetVars()["type"]
	// In the future, other authentication methods can be added here (e.g. Twitter)
	if authType == signInWithEthereumType {
		return linkWithSIWE(ctx, logger, nk, in)
	}
	return nil, ErrBadCustomAuthType
}
