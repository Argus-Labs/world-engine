package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	nakamaerrors "pkg.world.dev/world-engine/relay/nakama/errors"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"
	"strings"
	"sync"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/sign"
)

const (
	EnvCardinalAddr           = "CARDINAL_ADDR"
	EnvCardinalNamespace      = "CARDINAL_NAMESPACE"
	ListEndpoints             = "query/http/endpoints"
	EventEndpoint             = "events"
	TransactionEndpointPrefix = "/tx"
)

var (
	globalCardinalAddress      string
	globalNamespace            string
	globalPersonaTagAssignment = sync.Map{}
	globalReceiptsDispatcher   *receipt.ReceiptsDispatcher
)

func InitModule(
	ctx context.Context,
	logger runtime.Logger,
	_ *sql.DB,
	nk runtime.NakamaModule,
	initializer runtime.Initializer,
) error {
	utils.DebugEnabled = getDebugModeFromEnvironment()

	if err := initCardinalAddress(); err != nil {
		return eris.Wrap(err, "failed to init cardinal address")
	}

	if err := initNamespace(); err != nil {
		return eris.Wrap(err, "failed to init namespace")
	}

	initReceiptDispatcher(logger)

	if err := initEventHub(ctx, logger, nk); err != nil {
		return eris.Wrap(err, "failed to init event hub")
	}

	notifier := receipt.NewNotifier(logger, nk, globalReceiptsDispatcher)

	if err := signer.InitPrivateKey(ctx, logger, nk); err != nil {
		return eris.Wrap(err, "failed to init private key")
	}

	if err := initPersonaTagAssignmentMap(ctx, logger, nk, persona.CardinalCollection); err != nil {
		return eris.Wrap(err, "failed to init persona tag assignment map")
	}

	ptv := persona.NewVerifier(logger, nk, globalReceiptsDispatcher)

	if err := initPersonaTagEndpoints(logger, initializer, ptv, notifier); err != nil {
		return eris.Wrap(err, "failed to init persona tag endpoints")
	}

	if err := initCardinalEndpoints(logger, initializer, notifier); err != nil {
		return eris.Wrap(err, "failed to init cardinal endpoints")
	}

	if err := initAllowlist(logger, initializer); err != nil {
		return eris.Wrap(err, "failed to init allowlist endpoints")
	}

	if err := initSaveFileStorage(logger, initializer); err != nil {
		return eris.Wrap(err, "failed to init save file storage endpoint")
	}

	if err := initSaveFileQuery(logger, initializer); err != nil {
		return eris.Wrap(err, "failed to init save file query endpoint")
	}

	return nil
}

func initReceiptDispatcher(log runtime.Logger) {
	globalReceiptsDispatcher = receipt.NewReceiptsDispatcher()
	go globalReceiptsDispatcher.PollReceipts(log, globalCardinalAddress)
	go globalReceiptsDispatcher.Dispatch(log)
}

func initEventHub(ctx context.Context, log runtime.Logger, nk runtime.NakamaModule) error {
	eventHub, err := createEventHub(log)
	if err != nil {
		return err
	}
	go func() {
		err := eventHub.Dispatch(log)
		if err != nil {
			log.Error("error initializing eventHub: %s", eris.ToString(err, true))
		}
	}()

	// for now send to everybody via notifications.
	go func() {
		channel := eventHub.Subscribe("main")
		for event := range channel {
			err := eris.Wrap(nk.NotificationSendAll(ctx, "event", map[string]interface{}{"message": event.message}, 1, true), "")
			if err != nil {
				log.Error("error sending notifications: %s", eris.ToString(err, true))
			}
		}
	}()

	return nil
}

// initPersonaTagAssignmentMap initializes a sync.Map with all the existing mappings of PersonaTag->UserID. This
// sync.Map ensures that multiple users will not be given the same persona tag.
func initPersonaTagAssignmentMap(
	ctx context.Context,
	logger runtime.Logger,
	nk runtime.NakamaModule,
	collectionName string,
) error {
	logger.Debug("attempting to build personaTag->userID mapping")
	var cursor string
	var objs []*api.StorageObject
	var err error
	iterationLimit := 100
	for {
		objs, cursor, err = nk.StorageList(ctx, "", "", collectionName, iterationLimit, cursor)
		if err != nil {
			return eris.Wrap(err, "")
		}
		logger.Debug("found %d persona tag storage objects", len(objs))
		for _, obj := range objs {
			userID := obj.UserId
			var ptr *persona.StorageObj
			ptr, err = persona.StorageObjToPersonaTagStorageObj(obj)
			if err != nil {
				return err
			}
			if ptr.Status == persona.StatusAccepted || ptr.Status == persona.StatusPending {
				logger.Debug("%s has been assigned to %s", ptr.PersonaTag, userID)
				globalPersonaTagAssignment.Store(ptr.PersonaTag, userID)
			}
		}
		if cursor == "" {
			break
		}
	}
	return nil
}

// initCardinalEndpoints queries the cardinal server to find the list of existing endpoints, and attempts to
// set up RPC wrappers around each one.
//
//nolint:gocognit,funlen // its fine.
func initCardinalEndpoints(
	logger runtime.Logger,
	initializer runtime.Initializer,
	notify *receipt.Notifier,
) error {
	txEndpoints, queryEndpoints, err := getCardinalEndpoints()
	if err != nil {
		return err
	}

	createTransaction := func(payload string, endpoint string, nk runtime.NakamaModule, ctx context.Context,
	) (io.Reader, error) {
		logger.Debug("The %s endpoint requires a signed payload", endpoint)
		var transaction io.Reader
		transaction, err = makeTransaction(ctx, nk, payload)
		if err != nil {
			return nil, err
		}
		return transaction, nil
	}

	createUnsignedTransaction := func(payload string, endpoint string, _ runtime.NakamaModule, _ context.Context,
	) (io.Reader, error) {
		payloadBytes := []byte(payload)
		formattedPayloadBuffer := bytes.NewBuffer([]byte{})
		if !json.Valid(payloadBytes) {
			return nil, eris.Errorf("data %q is not valid json", string(payloadBytes))
		}
		err = json.Compact(formattedPayloadBuffer, payloadBytes)
		if err != nil {
			return nil, eris.Wrap(err, "")
		}
		return formattedPayloadBuffer, nil
	}

	registerEndpoints := func(endpoints []string, createPayload func(string, string, runtime.NakamaModule,
		context.Context) (io.Reader, error)) error {
		for _, e := range endpoints {
			logger.Debug("registering: %v", e)
			currEndpoint := e
			if currEndpoint[0] == '/' {
				currEndpoint = currEndpoint[1:]
			}
			err = initializer.RegisterRpc(currEndpoint, func(ctx context.Context, logger runtime.Logger, db *sql.DB,
				nk runtime.NakamaModule, payload string) (string, error) {
				logger.Debug("Got request for %q", currEndpoint)
				var resultPayload io.Reader
				resultPayload, err = createPayload(payload, currEndpoint, nk, ctx)
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
					notify.AddTxHashToPendingNotifications(asTx.TxHash, userID)
				}

				return string(bz), nil
			})
			if err != nil {
				return eris.Wrap(err, "")
			}
		}
		return nil
	}

	err = registerEndpoints(txEndpoints, createTransaction)
	if err != nil {
		return err
	}
	err = registerEndpoints(queryEndpoints, createUnsignedTransaction)
	if err != nil {
		return err
	}
	return nil
}

// setPersonaTagAssignment attempts to associate a given persona tag with the given user ID, and returns
// true if the attempt was successful or false if it failed. This method is safe for concurrent access.
func setPersonaTagAssignment(personaTag, userID string) (ok bool) {
	val, loaded := globalPersonaTagAssignment.LoadOrStore(personaTag, userID)
	if !loaded {
		return true
	}
	gotUserID, _ := val.(string)
	return gotUserID == userID
}

func makeTransaction(ctx context.Context, nk runtime.NakamaModule, payload string) (io.Reader, error) {
	ptr, err := persona.LoadPersonaTagStorageObj(ctx, nk)
	if err != nil {
		return nil, err
	}
	ptr, err = ptr.AttemptToUpdatePending(ctx, nk, globalCardinalAddress)
	if err != nil {
		return nil, err
	}

	if ptr.Status != persona.StatusAccepted {
		return nil, eris.Wrap(nakamaerrors.ErrNoPersonaTagForUser, "")
	}
	personaTag := ptr.PersonaTag
	pk, nonce, err := signer.GetPrivateKeyAndANonce(ctx, nk)
	if err != nil {
		return nil, err
	}
	sp, err := sign.NewTransaction(pk, personaTag, globalNamespace, nonce, payload)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(sp)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return bytes.NewReader(buf), nil
}

func initCardinalAddress() error {
	globalCardinalAddress = os.Getenv(EnvCardinalAddr)
	if globalCardinalAddress == "" {
		return eris.Errorf("must specify a cardinal server via %s", EnvCardinalAddr)
	}
	return nil
}

func initNamespace() error {
	globalNamespace = os.Getenv(EnvCardinalNamespace)
	if globalNamespace == "" {
		return eris.Errorf("must specify a cardinal namespace via %s", EnvCardinalNamespace)
	}
	return nil
}

func getDebugModeFromEnvironment() bool {
	devModeString := os.Getenv("ENABLE_DEBUG")
	return strings.ToLower(devModeString) == "true"
}
