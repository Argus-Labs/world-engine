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

	"pkg.world.dev/world-engine/relay/nakama/events"

	kms "cloud.google.com/go/kms/apiv1"
	"google.golang.org/api/option"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

const (
	EnvCardinalAddr           = "CARDINAL_ADDR"
	EnvCardinalNamespace      = "CARDINAL_NAMESPACE"
	EnvKMSCredentialsFile     = "GCP_KMS_CREDENTIALS_FILE" // #nosec G101
	EnvKMSKeyName             = "GCP_KMS_KEY_NAME"
	ListEndpoints             = "query/http/endpoints"
	EventEndpoint             = "events"
	TransactionEndpointPrefix = "tx/"
)

func InitModule(
	ctx context.Context,
	logger runtime.Logger,
	_ *sql.DB,
	nk runtime.NakamaModule,
	initializer runtime.Initializer,
) error {
	utils.DebugEnabled = getDebugModeFromEnvironment()

	cardinalAddress, err := initCardinalAddress()
	if err != nil {
		return eris.Wrap(err, "failed to init cardinal address")
	}

	globalNamespace, err := initNamespace()
	if err != nil {
		return eris.Wrap(err, "failed to init globalNamespace")
	}

	globalReceiptsDispatcher := initReceiptDispatcher(logger, cardinalAddress)

	if err := initEventHub(ctx, logger, nk, EventEndpoint, cardinalAddress); err != nil {
		return eris.Wrap(err, "failed to init event hub")
	}

	notifier := receipt.NewNotifier(logger, nk, globalReceiptsDispatcher)

	txSigner, err := selectSigner(ctx, logger, nk)
	if err != nil {
		return eris.Wrap(err, "failed to create a crypto signer")
	}

	globalPersonaAssignment := &sync.Map{}
	if err := initPersonaTagAssignmentMap(
		ctx,
		logger,
		nk,
		persona.CardinalCollection,
		globalPersonaAssignment,
	); err != nil {
		return eris.Wrap(err, "failed to init persona tag assignment map")
	}

	verifier := persona.NewVerifier(logger, nk, globalReceiptsDispatcher)

	if err := initPersonaTagEndpoints(
		logger,
		initializer,
		verifier,
		notifier,
		txSigner,
		cardinalAddress,
		globalNamespace,
		globalPersonaAssignment,
	); err != nil {
		return eris.Wrap(err, "failed to init persona tag endpoints")
	}

	if err := initCardinalEndpoints(
		logger,
		initializer,
		notifier,
		txSigner,
		cardinalAddress,
		globalNamespace,
	); err != nil {
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

func initReceiptDispatcher(log runtime.Logger, cardinalAddress string) *receipt.Dispatcher {
	globalReceiptsDispatcher := receipt.NewDispatcher()
	go globalReceiptsDispatcher.PollReceipts(log, cardinalAddress)
	go globalReceiptsDispatcher.Dispatch(log)
	return globalReceiptsDispatcher
}

func selectSigner(ctx context.Context, logger runtime.Logger, nk runtime.NakamaModule) (signer.Signer, error) {
	nonceManager := signer.NewNakamaNonceManager(nk)

	kmsCredsFile := os.Getenv(EnvKMSCredentialsFile)
	kmsKeyName := os.Getenv(EnvKMSKeyName)
	if kmsCredsFile == "" && kmsKeyName == "" {
		// Neither the KMS creds file nor the key name is set. Assume the user wants to store the private key on
		// Nakama's DB.
		return signer.NewNakamaSigner(ctx, logger, nk, nonceManager)
	}
	// At least one of the creds file or the key name is set. Assume the user wants to use KSM for signing transactions.
	if kmsCredsFile == "" || kmsKeyName == "" {
		// If either the credentials file or the key name is unset, KMS signing won't work, so return an error.
		return nil, eris.Errorf(
			"Both %q and %q must be set to use GCP KMS signing", EnvKMSCredentialsFile, EnvKMSKeyName)
	}

	client, err := kms.NewKeyManagementClient(ctx, option.WithCredentialsFile(kmsCredsFile))
	if err != nil {
		return nil, eris.Wrap(err, "failed to make KMS client")
	}
	return signer.NewKMSSigner(ctx, nonceManager, client, kmsKeyName)
}

func initEventHub(
	ctx context.Context,
	log runtime.Logger,
	nk runtime.NakamaModule,
	eventsEndpoint string,
	cardinalAddress string,
) error {
	eventHub, err := events.CreateEventHub(log, eventsEndpoint, cardinalAddress)
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
			err := eris.Wrap(
				nk.NotificationSendAll(ctx, "event", map[string]interface{}{"message": event.Message}, 1, false), "")
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
	globalPersonaAssignment *sync.Map,
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
				globalPersonaAssignment.Store(ptr.PersonaTag, userID)
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
	txSigner signer.Signer,
	cardinalAddress string,
	globalNamespace string,
) error {
	txEndpoints, queryEndpoints, err := getCardinalEndpoints(cardinalAddress)
	if err != nil {
		return err
	}

	createTransaction := func(payload string, endpoint string, nk runtime.NakamaModule, ctx context.Context,
	) (io.Reader, error) {
		logger.Debug("The %s endpoint requires a signed payload", endpoint)
		var transaction io.Reader
		transaction, err = makeTransaction(ctx, nk, txSigner, payload, cardinalAddress, globalNamespace)
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

	err = registerEndpoints(logger, initializer, notifier, txEndpoints, createTransaction, cardinalAddress)
	if err != nil {
		return err
	}
	err = registerEndpoints(logger, initializer, notifier, queryEndpoints, createUnsignedTransaction, cardinalAddress)
	if err != nil {
		return err
	}
	return nil
}

func makeTransaction(ctx context.Context,
	nk runtime.NakamaModule,
	txSigner signer.Signer,
	payload string,
	cardinalAddress string,
	globalNamespace string,
) (io.Reader, error) {
	ptr, err := persona.LoadPersonaTagStorageObj(ctx, nk)
	if err != nil {
		return nil, err
	}
	ptr, err = ptr.AttemptToUpdatePending(ctx, nk, txSigner, cardinalAddress)
	if err != nil {
		return nil, err
	}

	if ptr.Status != persona.StatusAccepted {
		return nil, eris.Wrap(persona.ErrNoPersonaTagForUser, "")
	}
	personaTag := ptr.PersonaTag
	sp, err := txSigner.SignTx(ctx, personaTag, globalNamespace, payload)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(sp)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return bytes.NewReader(buf), nil
}

func initCardinalAddress() (string, error) {
	globalCardinalAddress := os.Getenv(EnvCardinalAddr)
	if globalCardinalAddress == "" {
		return "", eris.Errorf("must specify a cardinal server via %s", EnvCardinalAddr)
	}
	return globalCardinalAddress, nil
}

func initNamespace() (string, error) {
	globalNamespace := os.Getenv(EnvCardinalNamespace)
	if globalNamespace == "" {
		return "", eris.Errorf("must specify a cardinal namespace via %s", EnvCardinalNamespace)
	}
	return globalNamespace, nil
}

func getDebugModeFromEnvironment() bool {
	devModeString := os.Getenv("ENABLE_DEBUG")
	return strings.ToLower(devModeString) == "true"
}
