package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"io"
	"net/http"
	"os"
	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"pkg.world.dev/world-engine/relay/nakama/utils"
	"strconv"
	"strings"
)

// initPersonaEndpoints sets up the nakame RPC endpoints that are used to claim a persona tag and display a persona tag.
func initPersonaTagEndpoints(
	_ runtime.Logger,
	initializer runtime.Initializer,
	ptv *persona.Verifier,
	notifier *receipt.Notifier) error {
	if err := initializer.RegisterRpc("nakama/claim-persona", handleClaimPersona(ptv, notifier)); err != nil {
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

//nolint:gocognit
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
		err := initializer.RegisterRpc(currEndpoint, func(ctx context.Context, logger runtime.Logger, db *sql.DB,
			nk runtime.NakamaModule, payload string) (string, error) {
			logger.Debug("Got request for %q", currEndpoint)
			var resultPayload io.Reader
			resultPayload, err := createPayload(payload, currEndpoint, nk, ctx)
			if err != nil {
				return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to make payload")
			}

			req, err := http.NewRequestWithContext(
				ctx,
				http.MethodPost,
				utils.MakeHTTPURL(currEndpoint, globalCardinalAddress),
				resultPayload,
			)
			req.Header.Set("Content-Type", "application/json")
			if err != nil {
				return utils.LogErrorMessageFailedPrecondition(logger, err, "request setup failed for endpoint %q", currEndpoint)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return utils.LogErrorMessageFailedPrecondition(logger, err, "request failed for endpoint %q", currEndpoint)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return utils.LogErrorMessageFailedPrecondition(
						logger,
						eris.Wrap(err, "failed to read response body"),
						"bad status code: %s: %s", resp.Status, body,
					)
				}
				return utils.LogErrorMessageFailedPrecondition(
					logger,
					eris.Errorf("bad status code %d", resp.StatusCode),
					"bad status code: %s: %s", resp.Status, body,
				)
			}
			bz, err := io.ReadAll(resp.Body)
			if err != nil {
				return utils.LogErrorMessageFailedPrecondition(logger, err, "can't read body")
			}
			if strings.HasPrefix(currEndpoint, TransactionEndpointPrefix) {
				var asTx persona.TxResponse

				if err = json.Unmarshal(bz, &asTx); err != nil {
					return utils.LogErrorMessageFailedPrecondition(logger, err, "can't decode body as tx response")
				}
				userID, err := utils.GetUserID(ctx)
				if err != nil {
					return utils.LogErrorMessageFailedPrecondition(logger, err, "unable to get user id")
				}
				notifier.AddTxHashToPendingNotifications(asTx.TxHash, userID)
			}

			return string(bz), nil
		})
		if err != nil {
			return eris.Wrap(err, "")
		}
	}
	return nil
}
