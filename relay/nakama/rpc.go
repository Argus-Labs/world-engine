package main

import (
	"context"
	"io"
	"os"
	"strconv"
	"sync"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/events"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/signer"
)

// initPersonaEndpoints sets up the nakame RPC endpoints that are used to claim a persona tag and display a persona tag.
func initPersonaTagEndpoints(
	_ runtime.Logger,
	initializer runtime.Initializer,
	verifier *persona.Verifier,
	notifier *events.Notifier,
	txSigner signer.Signer,
	cardinalAddress string,
	globalNamespace string,
	globalPersonaAssignment *sync.Map,
) error {
	err := initializer.RegisterRpc(
		"nakama/claim-persona",
		handleClaimPersona(
			verifier,
			notifier,
			txSigner,
			cardinalAddress,
			globalNamespace,
			globalPersonaAssignment,
		),
	)
	if err != nil {
		return eris.Wrap(err, "")
	}
	return eris.Wrap(initializer.RegisterRpc("nakama/show-persona", handleShowPersona(txSigner, cardinalAddress)), "")
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
	notifier *events.Notifier,
	endpoints []string,
	createPayload func(
		string, string, runtime.NakamaModule,
		context.Context,
	) (io.Reader, error),
	cardinalAddress string,
	namespace string,
	txSigner signer.Signer,
	autoReclaimPersonaTags bool,
) error {
	for _, e := range endpoints {
		logger.Debug("registering: %v", e)
		currEndpoint := e
		if currEndpoint[0] == '/' {
			currEndpoint = currEndpoint[1:]
		}
		requestHandler := handleCardinalRequest(
			currEndpoint,
			createPayload,
			notifier,
			cardinalAddress,
			namespace,
			txSigner,
			autoReclaimPersonaTags,
		)
		err := initializer.RegisterRpc(currEndpoint, requestHandler)
		if err != nil {
			return eris.Wrap(err, "")
		}
	}
	return nil
}
