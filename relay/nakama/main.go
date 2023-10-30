package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"pkg.world.dev/world-engine/sign"
)

const (
	OK                 = 0
	Cancelled          = 1
	Unknown            = 2
	InvalidArgument    = 3
	DeadlineExceeded   = 4
	NotFound           = 5
	AlreadyExists      = 6
	PermissionDenied   = 7
	ResourceExhausted  = 8
	FailedPrecondition = 9
	Aborted            = 10
	OutOfRange         = 11
	Unimplemented      = 12
	Internal           = 13
	Unavailable        = 14
	DataLoss           = 15
	Unauthenticated    = 16
)

type receiptChan chan *Receipt

const (
	EnvCardinalAddr      = "CARDINAL_ADDR"
	EnvCardinalNamespace = "CARDINAL_NAMESPACE"

	cardinalCollection = "cardinal_collection"
	personaTagKey      = "personaTag"

	transactionEndpointPrefix = "/tx"
)

var (
	ErrPersonaTagStorageObjNotFound = errors.New("persona tag storage object not found")
	ErrNoPersonaTagForUser          = errors.New("user does not have a verified persona tag")

	globalNamespace string

	globalPersonaTagAssignment = sync.Map{}

	globalReceiptsDispatcher *receiptsDispatcher
)

func InitModule(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	initializer runtime.Initializer,
) error {
	if err := initCardinalAddress(); err != nil {
		return fmt.Errorf("failed to init cardinal address: %w", err)
	}

	if err := initNamespace(); err != nil {
		return fmt.Errorf("failed to init namespace: %w", err)
	}

	initReceiptDispatcher(logger)

	if err := initEventHub(ctx, logger, nk); err != nil {
		return fmt.Errorf("failed to init event hub: %w", err)
	}

	if err := initReceiptMatch(ctx, logger, db, nk, initializer); err != nil {
		return fmt.Errorf("unable to init match for receipt streaming: %w", err)
	}

	notifier := newReceiptNotifier(logger, nk)

	if err := initPrivateKey(ctx, logger, nk); err != nil {
		return fmt.Errorf("failed to init private key: %w", err)
	}

	if err := initPersonaTagAssignmentMap(ctx, logger, nk); err != nil {
		return fmt.Errorf("failed to init persona tag assignment map: %w", err)
	}

	ptv := initPersonaTagVerifier(logger, nk, globalReceiptsDispatcher)

	if err := initPersonaTagEndpoints(logger, initializer, ptv, notifier); err != nil {
		return fmt.Errorf("failed to init persona tag endpoints: %w", err)
	}

	if err := initCardinalEndpoints(logger, initializer, notifier); err != nil {
		return fmt.Errorf("failed to init cardinal endpoints: %w", err)
	}

	return nil
}

func initNamespace() error {
	globalNamespace = os.Getenv(EnvCardinalNamespace)
	if globalNamespace == "" {
		return fmt.Errorf("must specify a cardinal namespace via %s", EnvCardinalNamespace)
	}
	return nil
}

func initReceiptDispatcher(log runtime.Logger) {
	globalReceiptsDispatcher = newReceiptsDispatcher()
	go globalReceiptsDispatcher.pollReceipts(log)
	go globalReceiptsDispatcher.dispatch(log)
}

func initEventHub(ctx context.Context, log runtime.Logger, nk runtime.NakamaModule) error {
	eventHub, err := createEventHub(log)
	if err != nil {
		return err
	}
	go func() {
		err := eventHub.dispatch(log)
		if err != nil {
			log.Error("error initializing eventHub: %s", err.Error())
		}
	}()

	// for now send to everybody via notifications.
	go func() {
		channel := eventHub.subscribe("main")
		for event := range channel {
			err := nk.NotificationSendAll(ctx, "event", map[string]interface{}{"message": event.message}, 1, true)
			if err != nil {
				log.Error("error sending notifications: %s", err.Error())
			}
		}
	}()

	return nil
}

func initReceiptMatch(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule,
	initializer runtime.Initializer) error {
	err := initializer.RegisterMatch("lobby", func(ctx context.Context, logger runtime.Logger, db *sql.DB,
		nk runtime.NakamaModule) (runtime.Match, error) {
		return &ReceiptMatch{}, nil
	})
	if err != nil {
		logger.Error("unable to register match: %v", err)
		return err
	}
	result, err := nk.MatchCreate(ctx, "lobby", map[string]any{})
	if err != nil {
		logger.Error("unable to create match: %v", err)
		return err
	}
	logger.Debug("match create result is %q", result)
	return nil
}

// initPersonaTagAssignmentMap initializes a sync.Map with all the existing mappings of PersonaTag->UserID. This
// sync.Map ensures that multiple users will not be given the same persona tag.
func initPersonaTagAssignmentMap(ctx context.Context, logger runtime.Logger, nk runtime.NakamaModule) error {
	logger.Debug("attempting to build personaTag->userID mapping")
	var cursor string
	var objs []*api.StorageObject
	var err error
	iterationLimit := 100
	for {
		objs, cursor, err = nk.StorageList(ctx, "", cardinalCollection, iterationLimit, cursor)
		if err != nil {
			return err
		}
		logger.Debug("found %d persona tag storage objects", len(objs))
		for _, obj := range objs {
			userID := obj.UserId
			var ptr *personaTagStorageObj
			ptr, err = storageObjToPersonaTagStorageObj(obj)
			if err != nil {
				return err
			}
			if ptr.Status == personaTagStatusAccepted || ptr.Status == personaTagStatusPending {
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

// initPersonaEndpoints sets up the nakame RPC endpoints that are used to claim a persona tag and display a persona tag.
func initPersonaTagEndpoints(
	_ runtime.Logger,
	initializer runtime.Initializer,
	ptv *personaTagVerifier,
	notifier *receiptNotifier) error {
	if err := initializer.RegisterRpc("nakama/claim-persona", handleClaimPersona(ptv, notifier)); err != nil {
		return err
	}
	return initializer.RegisterRpc("nakama/show-persona", handleShowPersona)
}

// getUserID gets the Nakama UserID from the given context.
func getUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	if !ok {
		return "", errors.New("unable to get user id from context")
	}
	return userID, nil
}

// nakamaRPCHandler is the signature required for handlers that are passed to Nakama's RegisterRpc method.
// This type is defined just to make the function below a little more readable.
type nakamaRPCHandler func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule,
	payload string) (string, error)

// handleClaimPersona handles a request to Nakama to associate the current user with the persona tag in the payload.
//
//nolint:gocognit
func handleClaimPersona(ptv *personaTagVerifier, notifier *receiptNotifier) nakamaRPCHandler {
	return func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (
		string, error) {
		if ptr, err := loadPersonaTagStorageObj(ctx, nk); err != nil && !errors.Is(err, ErrPersonaTagStorageObjNotFound) {
			return logError(logger, "unable to get persona tag storage object: %w", err)
		} else if err == nil {
			switch ptr.Status {
			case personaTagStatusPending:
				return logCode(logger, AlreadyExists, "persona tag %q is pending for this account", ptr.PersonaTag)
			case personaTagStatusAccepted:
				return logCode(logger, AlreadyExists, "persona tag %q already associated with this account", ptr.PersonaTag)
			case personaTagStatusRejected:
				return logCode(logger, AlreadyExists, "persona tag %q rejected", ptr.PersonaTag)
			default:
				// In other cases, allow the user to claim a persona tag.
			}
		}

		ptr := &personaTagStorageObj{}
		if err := json.Unmarshal([]byte(payload), ptr); err != nil {
			return logError(logger, "unable to marshal payload: %w", err)
		}
		if ptr.PersonaTag == "" {
			return logCode(logger, InvalidArgument, "personaTag field must not be empty")
		}

		userID, err := getUserID(ctx)
		if err != nil {
			return logError(logger, "unable to get userID: %w", err)
		}
		txHash, tick, err := cardinalCreatePersona(ctx, nk, ptr.PersonaTag)
		if err != nil {
			return logError(logger, "unable to make create persona request to cardinal: %v", err)
		}
		notifier.AddTxHashToPendingNotifications(txHash, userID)

		ptr.Status = personaTagStatusPending
		if err = ptr.savePersonaTagStorageObj(ctx, nk); err != nil {
			return logError(logger, "unable to set persona tag storage object: %w", err)
		}

		// Try to actually assign this personaTag->UserID in the sync map. If this succeeds, Nakama is OK with this
		// user having the persona tag.
		if ok := setPersonaTagAssignment(ptr.PersonaTag, userID); !ok {
			ptr.Status = personaTagStatusRejected
			if err = ptr.savePersonaTagStorageObj(ctx, nk); err != nil {
				return logError(logger, "unable to set persona tag storage object: %v", err)
			}
			return logCode(logger, AlreadyExists, "persona tag %q is not available", ptr.PersonaTag)
		}

		ptr.Tick = tick
		ptr.TxHash = txHash
		if err = ptr.savePersonaTagStorageObj(ctx, nk); err != nil {
			return logError(logger, "unable to save persona tag storage object: %v", err)
		}
		ptv.addPendingPersonaTag(userID, ptr.TxHash)
		return ptr.toJSON()
	}
}

func handleShowPersona(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, _ string,
) (string, error) {
	ptr, err := loadPersonaTagStorageObj(ctx, nk)
	if errors.Is(err, ErrPersonaTagStorageObjNotFound) {
		return logError(logger, "no persona tag found: %w", err)
	} else if err != nil {
		return logError(logger, "unable to get persona tag storage object: %w", err)
	}
	ptr, err = ptr.attemptToUpdatePending(ctx, nk)
	if err != nil {
		return logError(logger, "unable to update pending state: %v", err)
	}
	return ptr.toJSON()
}

// initCardinalEndpoints queries the cardinal server to find the list of existing endpoints, and attempts to
// set up RPC wrappers around each one.
//
//nolint:gocognit
func initCardinalEndpoints(logger runtime.Logger, initializer runtime.Initializer, notify *receiptNotifier) error {
	txEndpoints, queryEndpoints, err := getCardinalEndpoints()
	if err != nil {
		return err
	}

	createSignedPayload := func(payload string, endpoint string, nk runtime.NakamaModule, ctx context.Context,
	) (io.Reader, error) {
		logger.Debug("The %s endpoint requires a signed payload", endpoint)
		var signedPayload io.Reader
		signedPayload, err = makeSignedPayload(ctx, nk, payload)
		if err != nil {
			return nil, err
		}
		return signedPayload, nil
	}

	createUnsignedPayload := func(payload string, endpoint string, _ runtime.NakamaModule, _ context.Context,
	) (io.Reader, error) {
		payloadBytes := []byte(payload)
		formattedPayloadBuffer := bytes.NewBuffer([]byte{})
		if !json.Valid(payloadBytes) {
			return nil, fmt.Errorf("data %q is not valid json", string(payloadBytes))
		}
		err = json.Compact(formattedPayloadBuffer, payloadBytes)
		if err != nil {
			return nil, err
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
					return logError(logger, "unable to make payload: %w", err)
				}

				req, err := http.NewRequestWithContext(ctx, http.MethodPost, makeHTTPURL(currEndpoint), resultPayload)
				req.Header.Set("Content-Type", "application/json")
				if err != nil {
					return logError(logger, "request setup failed for endpoint %q: %w", currEndpoint, err)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return logError(logger, "request failed for endpoint %q: %w", currEndpoint, err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					body, _ := io.ReadAll(resp.Body)
					return logError(logger, "bad status code: %s: %s", resp.Status, body)
				}
				bz, err := io.ReadAll(resp.Body)
				if err != nil {
					return logError(logger, "can't read body: %w", err)
				}
				if strings.HasPrefix(currEndpoint, transactionEndpointPrefix) {
					var asTx txResponse

					if err = json.Unmarshal(bz, &asTx); err != nil {
						return logError(logger, "can't decode body as tx response: %w", err)
					}
					userID, err := getUserID(ctx)
					if err != nil {
						return logError(logger, "unable to get user id: %w", err)
					}
					notify.AddTxHashToPendingNotifications(asTx.TxHash, userID)
				}

				return string(bz), nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	}

	err = registerEndpoints(txEndpoints, createSignedPayload)
	if err != nil {
		return err
	}
	err = registerEndpoints(queryEndpoints, createUnsignedPayload)
	if err != nil {
		return err
	}
	return nil
}

func logCode(logger runtime.Logger, code int, format string, v ...interface{}) (string, error) {
	err := fmt.Errorf(format, v...)
	logger.Error(err.Error())
	return "", runtime.NewError(err.Error(), code)
}

func logError(logger runtime.Logger, format string, v ...interface{}) (string, error) {
	err := fmt.Errorf(format, v...)
	logger.Error(err.Error())
	return "", runtime.NewError(err.Error(), Internal)
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

func makeSignedPayload(ctx context.Context, nk runtime.NakamaModule, payload string) (io.Reader, error) {
	ptr, err := loadPersonaTagStorageObj(ctx, nk)
	if err != nil {
		return nil, err
	}
	ptr, err = ptr.attemptToUpdatePending(ctx, nk)
	if err != nil {
		return nil, err
	}

	if ptr.Status != personaTagStatusAccepted {
		return nil, ErrNoPersonaTagForUser
	}
	personaTag := ptr.PersonaTag
	pk, nonce, err := getPrivateKeyAndANonce(ctx, nk)
	if err != nil {
		return nil, err
	}
	sp, err := sign.NewSignedPayload(pk, personaTag, globalNamespace, nonce, payload)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(sp)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(buf), nil
}
