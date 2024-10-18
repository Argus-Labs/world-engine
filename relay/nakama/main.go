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

	kms "cloud.google.com/go/kms/apiv1"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"google.golang.org/api/option"

	"pkg.world.dev/world-engine/relay/nakama/auth"
	"pkg.world.dev/world-engine/relay/nakama/events"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

const (
	EnvCardinalAddr           = "CARDINAL_ADDR"
	EnvCardinalNamespace      = "CARDINAL_NAMESPACE"
	EnvKMSCredentialsFile     = "GCP_KMS_CREDENTIALS_FILE" // #nosec G101
	EnvKMSKeyName             = "GCP_KMS_KEY_NAME"
	EnvTraceEnabled           = "TRACE_ENABLED"
	EnvJaegerAddr             = "JAEGER_ADDR"
	EnvJaegerSampleRate       = "JAEGER_SAMPLE_RATE"
	WorldEndpoint             = "world"
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

	// don't defer shutdown here, as it would shutdown otel immediately as it was initialized. we
	// don't need to handle shutdown as nakama is supposed to be a long running process and is only
	// stopped if the container itself is stopped (to be replaced by a new one).
	_, err := initOtelSDK(ctx, logger)
	if err != nil {
		return eris.Wrap(err, "failed to init otel sdk")
	}
	logger.Info("Initialized OpenTelemetry SDK")

	cardinalAddress, err := initCardinalAddress()
	if err != nil {
		return eris.Wrap(err, "failed to init cardinal address")
	}

	globalNamespace, err := initNamespace()
	if err != nil {
		return eris.Wrap(err, "failed to init globalNamespace")
	}

	eventHub, err := initEventHub(ctx, logger, nk, EventEndpoint, cardinalAddress)
	if err != nil {
		return eris.Wrap(err, "failed to init event hub")
	}

	notifier := events.NewNotifier(logger, nk, eventHub)

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

	verifier := persona.NewVerifier(logger, nk, eventHub)

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
		eventHub,
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

	if err := auth.InitCustomAuthentication(initializer); err != nil {
		return eris.Wrap(err, "failed to init ethereum authentication")
	}

	if err := auth.InitCustomLink(initializer); err != nil {
		return eris.Wrap(err, "failed to init ethereum link")
	}
	return nil
}

func selectSigner(ctx context.Context, logger runtime.Logger, nk runtime.NakamaModule) (signer.Signer, error) {
	kmsCredsFile := os.Getenv(EnvKMSCredentialsFile)
	kmsKeyName := os.Getenv(EnvKMSKeyName)
	if kmsCredsFile == "" && kmsKeyName == "" {
		// Neither the KMS creds file nor the key name is set. Assume the user wants to store the private key on
		// Nakama's DB.
		return signer.NewNakamaSigner(ctx, logger, nk)
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
	return signer.NewKMSSigner(ctx, client, kmsKeyName)
}

func initEventHub(
	ctx context.Context,
	log runtime.Logger,
	nk runtime.NakamaModule,
	eventsEndpoint string,
	cardinalAddress string,
) (*events.EventHub, error) {
	eventHub, err := events.NewEventHub(log, eventsEndpoint, cardinalAddress)
	if err != nil {
		return nil, err
	}
	go func() {
		err := eventHub.Dispatch(log)
		if err != nil {
			log.Error("error initializing eventHub: %s", eris.ToString(err, true))
		}
	}()

	// Send Events to everyone via Nakama Notifications
	go func() {
		ch := eventHub.SubscribeToEvents("main")
		for event := range ch {
			content := make(map[string]any)
			err := json.Unmarshal(event, &content)
			if err != nil {
				// The event content isn't in JSON format. Wrap whatever it is in a JSON blob.
				content["message"] = string(event)
			}

			err = eris.Wrap(nk.NotificationSendAll(ctx, "event", content, 1, false), "")
			if err != nil {
				log.Error("error sending notifications: %s", eris.ToString(err, true))
			}
		}
	}()

	return eventHub, nil
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
			userID := obj.GetUserId()
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
	notifier *events.Notifier,
	eventHub *events.EventHub,
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

	createUnsignedTransaction := func(payload string, _ string, _ runtime.NakamaModule, _ context.Context,
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

	// Register all the transaction endpoints. These require signatures.
	err = registerEndpoints(
		logger,
		initializer,
		notifier,
		eventHub,
		txEndpoints,
		createTransaction,
		cardinalAddress,
		globalNamespace,
		txSigner,
		true,
	)
	if err != nil {
		return err
	}
	// Register all the query endpoints. These do not require signatures.
	// cql and debug/state are similar to normal cardinal queries, but they are not created by the same mechanism,
	// so they don't show up in the queryEndpoints slice.
	queryEndpoints = append(queryEndpoints, "cql", "debug/state")
	err = registerEndpoints(
		// Register all the transaction endpoints. These require signatures.
		logger,
		initializer,
		notifier,
		eventHub,
		queryEndpoints,
		createUnsignedTransaction,
		cardinalAddress,
		globalNamespace,
		txSigner,
		false)
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
