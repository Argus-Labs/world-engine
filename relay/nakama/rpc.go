package main

import (
	"context"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"io"
	"os"
	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"strconv"
)

// initPersonaEndpoints sets up the nakame RPC endpoints that are used to claim a persona tag and display a persona tag.
func initPersonaTagEndpoints(
	_ runtime.Logger,
	initializer runtime.Initializer,
	verifier *persona.Verifier,
	notifier *receipt.Notifier,
) error {
	if err := initializer.RegisterRpc("nakama/claim-persona", handleClaimPersona(verifier, notifier)); err != nil {
		return eris.Wrap(err, "")
	}
	return eris.Wrap(initializer.RegisterRpc("nakama/show-persona", handleShowPersona), "")
}

func initAllowlist(_ runtime.Logger, initializer runtime.Initializer) error {
	enabledStr := os.Getenv(allowlist.EnabledEnvVar)
	if enabledStr == "" {
		return nil
	}
	var err error
	allowlist.Enabled, err = strconv.ParseBool(enabledStr)
	if err != nil {
		return eris.Wrapf(err, "the ENABLE_ALLOWLIST flag was set, however the variable %q was an invalid ", enabledStr)
	}

	if !allowlist.Enabled {
		return nil
	}
	err = initializer.RegisterRpc("generate-beta-keys", handleGenerateKey)
	if err != nil {
		return eris.Wrap(err, "failed to register rpc")
	}

	err = initializer.RegisterRpc("claim-key", handleClaimKey)
	if err != nil {
		return eris.Wrap(err, "failed to register rpc")
	}
	return nil
}

func initSaveFileStorage(_ runtime.Logger, initializer runtime.Initializer) error {
	err := initializer.RegisterRpc(
		"nakama/save",
		handleSaveGame,
	)
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}

func initSaveFileQuery(_ runtime.Logger, initializer runtime.Initializer) error {
	err := initializer.RegisterRpc(
		"nakama/get-save",
		handleGetSaveGame,
	)
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}

func registerEndpoints(
	logger runtime.Logger,
	initializer runtime.Initializer,
	notifier *receipt.Notifier,
	endpoints []string,
	createPayload func(string, string, runtime.NakamaModule,
		context.Context,
	) (io.Reader, error)) error {
	for _, e := range endpoints {
		logger.Debug("registering: %v", e)
		currEndpoint := e
		if currEndpoint[0] == '/' {
			currEndpoint = currEndpoint[1:]
		}
		err := initializer.RegisterRpc(currEndpoint, handleCardinalRequest(currEndpoint, createPayload, notifier))
		if err != nil {
			return eris.Wrap(err, "")
		}
	}
	return nil
}
