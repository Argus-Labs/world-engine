package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"

	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"

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

	if err := initEventHub(ctx, logger, nk, globalCardinalAddress); err != nil {
		return eris.Wrap(err, "failed to init event hub")
	}

	notifier := receipt.NewNotifier(logger, nk, globalReceiptsDispatcher)

	if err := signer.InitPrivateKey(ctx, logger, nk); err != nil {
		return eris.Wrap(err, "failed to init private key")
	}

	if err := initPersonaTagAssignmentMap(ctx, logger, nk, persona.CardinalCollection); err != nil {
		return eris.Wrap(err, "failed to init persona tag assignment map")
	}

	verifier := persona.NewVerifier(logger, nk, globalReceiptsDispatcher)

	if err := initPersonaTagEndpoints(logger, initializer, verifier, notifier); err != nil {
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

func initEventHub(
	ctx context.Context,
	log runtime.Logger,
	nk runtime.NakamaModule,
	cardinalAddress string,
) error {
	eventHub, err := createEventHub(log, cardinalAddress)
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
func initCardinalEndpoints(
	logger runtime.Logger,
	initializer runtime.Initializer,
	notifier *receipt.Notifier,
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

	err = registerEndpoints(logger, initializer, notifier, txEndpoints, createTransaction)
	if err != nil {
		return err
	}
	err = registerEndpoints(logger, initializer, notifier, queryEndpoints, createUnsignedTransaction)
	if err != nil {
		return err
	}
	return nil
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
		return nil, eris.Wrap(persona.ErrNoPersonaTagForUser, "")
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
