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
	"sync"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"pkg.world.dev/world-engine/sign"
)

const (
	OK                  = 0
	CANCELED            = 1
	UNKNOWN             = 2
	INVALID_ARGUMENT    = 3
	DEADLINE_EXCEEDED   = 4
	NOT_FOUND           = 5
	ALREADY_EXISTS      = 6
	PERMISSION_DENIED   = 7
	RESOURCE_EXHAUSTED  = 8
	FAILED_PRECONDITION = 9
	ABORTED             = 10
	OUT_OF_RANGE        = 11
	UNIMPLEMENTED       = 12
	INTERNAL            = 13
	UNAVAILABLE         = 14
	DATA_LOSS           = 15
	UNAUTHENTICATED     = 16
)

type receiptChan chan *Receipt

const (
	EnvCardinalAddr      = "CARDINAL_ADDR"
	EnvCardinalNamespace = "CARDINAL_NAMESPACE"

	cardinalCollection = "cardinal_collection"
	personaTagKey      = "persona_tag"
)

var (
	ErrorPersonaTagStorageObjNotFound = errors.New("persona tag storage object not found")
	ErrorNoPersonaTagForUser          = errors.New("user does not have a verified persona tag")

	globalNamespace string

	globalPersonaTagAssignment = sync.Map{}

	globalReceiptsDispatcher *receiptsDispatcher
)

func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {

	if err := initCardinalAddress(); err != nil {
		return fmt.Errorf("failed to init cardinal address: %w", err)
	}

	if err := initNamespace(); err != nil {
		return fmt.Errorf("failed to init namespace: %w", err)
	}

	if err := initReceiptDispatcher(logger); err != nil {
		return fmt.Errorf("failed to init receipt dispatcher: %w", err)
	}

	if err := initReceiptMatch(ctx, logger, db, nk, initializer); err != nil {
		return fmt.Errorf("unable to init matches for receipt streaming")
	}

	if err := initPrivateKey(ctx, logger, nk); err != nil {
		return fmt.Errorf("failed to init private key: %w", err)
	}

	if err := initPersonaTagAssignmentMap(ctx, logger, nk); err != nil {
		return fmt.Errorf("failed to init persona tag assignment map: %w", err)
	}

	ptv, err := initPersonaTagVerifier(logger, nk, globalReceiptsDispatcher)
	if err != nil {
		return fmt.Errorf("failed to init persona tag verifier: %w", err)
	}

	if err := initPersonaTagEndpoints(logger, initializer, ptv); err != nil {
		return fmt.Errorf("failed to init persona tag endpoints: %w", err)
	}

	if err := initCardinalEndpoints(logger, initializer); err != nil {
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

func initReceiptDispatcher(log runtime.Logger) error {
	globalReceiptsDispatcher = newReceiptsDispatcher()
	go globalReceiptsDispatcher.pollReceipts(log)
	go globalReceiptsDispatcher.dispatch(log)
	return nil
}

func initReceiptMatch(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	err := initializer.RegisterMatch("lobby", func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule) (runtime.Match, error) {
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
	for {
		objs, cursor, err = nk.StorageList(ctx, "", cardinalCollection, 100, cursor)
		if err != nil {
			return err
		}
		logger.Debug("found %d persona tag storage objects", len(objs))
		for _, obj := range objs {
			userID := obj.UserId
			ptr, err := storageObjToPersonaTagStorageObj(obj)
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
func initPersonaTagEndpoints(logger runtime.Logger, initializer runtime.Initializer, ptv *personaTagVerifier) error {
	if err := initializer.RegisterRpc("nakama/claim-persona", handleClaimPersona(ptv)); err != nil {
		return err
	}
	if err := initializer.RegisterRpc("nakama/show-persona", handleShowPersona); err != nil {
		return err
	}
	return nil
}

// getUserID gets the Nakama UserID from the given context
func getUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	if !ok {
		return "", errors.New("unable to get user id from context")
	}
	return userID, nil
}

// nakamaRPCHandler is the signature required for handlers that are passed to Nakama's RegisterRpc method.
// This type is defined just to make the function below a little more readable.
type nakamaRPCHandler func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error)

// handleClaimPersona handles a request to Nakama to associate the current user with the persona tag in the payload.
func handleClaimPersona(ptv *personaTagVerifier) nakamaRPCHandler {
	return func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
		if ptr, err := loadPersonaTagStorageObj(ctx, nk); err != nil && err != ErrorPersonaTagStorageObjNotFound {
			return logError(logger, "unable to get persona tag storage object: %w", err)
		} else if err == nil {
			switch ptr.Status {
			case personaTagStatusPending:
				return logCode(logger, ALREADY_EXISTS, "persona tag %q is pending for this account", ptr.PersonaTag)
			case personaTagStatusAccepted:
				return logCode(logger, ALREADY_EXISTS, "persona tag %q already associated with this account", ptr.PersonaTag)
			default:
				// In other cases, allow the user to claim a persona tag.
			}
		}

		ptr := &personaTagStorageObj{}
		if err := json.Unmarshal([]byte(payload), ptr); err != nil {
			return logError(logger, "unable to marshal payload: %w", err)
		}
		if ptr.PersonaTag == "" {
			return logCode(logger, INVALID_ARGUMENT, "persona_tag field must not be empty")
		}

		ptr.Status = personaTagStatusPending
		if err := ptr.savePersonaTagStorageObj(ctx, nk); err != nil {
			return logError(logger, "unable to set persona tag storage object: %w", err)
		}

		userID, err := getUserID(ctx)
		if err != nil {
			return logError(logger, "unable to get userID: %w", err)
		}

		// Try to actually assign this personaTag->UserID in the sync map. If this succeeds, Nakama is OK with this
		// user having the persona tag. This assignment still needs to be checked with cardinal.
		if ok := setPersonaTagAssignment(ptr.PersonaTag, userID); !ok {
			ptr.Status = personaTagStatusRejected
			if err := ptr.savePersonaTagStorageObj(ctx, nk); err != nil {
				return logError(logger, "unable to set persona tag storage object: %v", err)
			}
			return logCode(logger, ALREADY_EXISTS, "persona tag %q is not available", ptr.PersonaTag)
		}

		txHash, tick, err := cardinalCreatePersona(ctx, nk, ptr.PersonaTag)
		if err != nil {
			return logError(logger, "unable to make create persona request to cardinal: %v", err)
		}

		ptr.Tick = tick
		ptr.TxHash = txHash
		if err := ptr.savePersonaTagStorageObj(ctx, nk); err != nil {
			return logError(logger, "unable to save persona tag storage object: %v", err)
		}
		ptv.addPendingPersonaTag(userID, ptr.TxHash)
		return ptr.toJSON()
	}
}

// verifyPersonaTag makes a request to Cardinal to see if this Nakama instance actually owns the given persona tag.
func verifyPersonaTag(ctx context.Context, ptr *personaTagStorageObj) (verified bool, err error) {
	gameSignerAddress, err := cardinalQueryPersonaSigner(ctx, ptr.PersonaTag, ptr.Tick)
	if err != nil {
		return false, err
	}
	nakamaSignerAddress := getSignerAddress()
	return gameSignerAddress == nakamaSignerAddress, nil
}

func handleShowPersona(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	ptr, err := loadPersonaTagStorageObj(ctx, nk)
	if errors.Is(err, ErrorPersonaTagStorageObjNotFound) {
		return logError(logger, "no persona tag found: %w", err)
	} else if err != nil {
		return logError(logger, "unable to get persona tag storage object: %w", err)
	}
	ptr, err = ptr.attemptToUpdatePending(ctx, nk)
	if err != nil {
		logError(logger, "unable to update pending state: %v", err)
	}
	return ptr.toJSON()
}

// initCardinalEndpoints queries the cardinal server to find the list of existing endpoints, and attempts to
// set up RPC wrappers around each one.
func initCardinalEndpoints(logger runtime.Logger, initializer runtime.Initializer) error {
	txEndpoints, queryEndpoints, err := cardinalGetEndpointsStruct()
	if err != nil {
		return err
	}

	createSignedPayload := func(payload string, endpoint string, nk runtime.NakamaModule, ctx context.Context) (io.Reader, error) {
		logger.Debug("The %s endpoint requires a signed payload", endpoint)
		signedPayload, err := makeSignedPayload(ctx, nk, payload)
		if err != nil {
			return nil, err
		}
		return signedPayload, nil
	}

	createUnsignedPayload := func(payload string, endpoint string, _ runtime.NakamaModule, _ context.Context) (io.Reader, error) {
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

	registerEndpoints := func(endpoints []string, createPayload func(string, string, runtime.NakamaModule, context.Context) (io.Reader, error)) error {
		for _, e := range endpoints {
			logger.Debug("registering: %v", e)
			currEndpoint := e
			if currEndpoint[0] == '/' {
				currEndpoint = currEndpoint[1:]
			}
			err := initializer.RegisterRpc(currEndpoint, func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
				logger.Debug("Got request for %q", currEndpoint)
				var resultPayload io.Reader
				resultPayload, err = createPayload(payload, currEndpoint, nk, ctx)
				if err != nil {
					return logError(logger, "unable to make payload: %w", err)
				}

				req, err := http.NewRequestWithContext(ctx, "POST", makeURL(currEndpoint), resultPayload)
				req.Header.Set("Content-Type", "application/json")
				if err != nil {
					return logError(logger, "request setup failed for endpoint %q: %w", currEndpoint, err)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return logError(logger, "request failed for endpoint %q: %w", currEndpoint, err)
				}
				if resp.StatusCode != 200 {
					body, _ := io.ReadAll(resp.Body)
					return logError(logger, "bad status code: %w: %s", resp.Status, body)
				}
				bodyStr, err := io.ReadAll(resp.Body)
				if err != nil {
					return logError(logger, "can't read body: %w", err)
				}
				return string(bodyStr), nil
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
	return "", runtime.NewError(err.Error(), INTERNAL)
}

// setPersonaTagAssignment attempts to associate a given persona tag with the given user ID, and returns
// true if the attempt was successful or false if it failed. This method is safe for concurrent access.
func setPersonaTagAssignment(personaTag, userID string) (ok bool) {
	val, loaded := globalPersonaTagAssignment.LoadOrStore(personaTag, userID)
	if !loaded {
		return true
	}
	gotUserID := val.(string)
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
		return nil, ErrorNoPersonaTagForUser
	}
	personaTag := ptr.PersonaTag
	pk, nonce, err := getPrivateKeyAndANonce(ctx, nk)
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
