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

	"github.com/argus-labs/world-engine/sign"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
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

type personaTagStatus string

const (
	personaTagStatusUnknown  personaTagStatus = "unknown"
	personaTagStatusPending  personaTagStatus = "pending"
	personaTagStatusAccepted personaTagStatus = "accepted"
	personaTagStatusRejected personaTagStatus = "rejected"

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
)

func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {

	if err := initCardinalAddress(); err != nil {
		return err
	}

	if err := initNamespace(); err != nil {
		return err
	}

	if err := initPrivateKey(ctx, logger, nk); err != nil {
		return fmt.Errorf("failed to init private key: %w", err)
	}

	if err := initPersonaTagAssignmentMap(ctx, logger, nk); err != nil {
		return fmt.Errorf("failed to init persona tag assignment map: %w", err)
	}

	if err := initPersonaEndpoints(logger, initializer); err != nil {
		return fmt.Errorf("failed to init persona endpoints: %w", err)
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
func initPersonaEndpoints(logger runtime.Logger, initializer runtime.Initializer) error {
	if err := initializer.RegisterRpc("nakama/claim-persona", handleClaimPersona); err != nil {
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

type personaTagStorageObj struct {
	PersonaTag string           `json:"persona_tag"`
	Status     personaTagStatus `json:"status"`
	Tick       uint64           `json:"tick"`
}

// storageObjToPersonaTagStorageObj converts a generic Nakama StorageObject to a locally defined personaTagStorageObj.
func storageObjToPersonaTagStorageObj(obj *api.StorageObject) (*personaTagStorageObj, error) {
	var ptr personaTagStorageObj
	if err := json.Unmarshal([]byte(obj.Value), &ptr); err != nil {
		return nil, fmt.Errorf("unable to unmarshal persona tag storage obj: %w", err)
	}
	return &ptr, nil
}

// getPersonaTag returns the persona tag (if any) associated with this user. ErrorNoPersonaTagForUser is returned
// if the user does not currently have a persona tag assigned.
func getPersonaTag(ctx context.Context, nk runtime.NakamaModule) (string, error) {
	ptr, err := getPersonaTagStorageObj(ctx, nk)
	if err != nil {
		return "", err
	}
	if ptr.Status != personaTagStatusAccepted {
		return "", ErrorNoPersonaTagForUser
	}
	return ptr.PersonaTag, nil
}

func getPersonaTagStorageObj(ctx context.Context, nk runtime.NakamaModule) (*personaTagStorageObj, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}
	storeObjs, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{
			Collection: cardinalCollection,
			Key:        personaTagKey,
			UserID:     userID,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(storeObjs) == 0 {
		return nil, ErrorPersonaTagStorageObjNotFound
	} else if len(storeObjs) > 1 {
		return nil, fmt.Errorf("expected 1 storage object, got %d with values %v", len(storeObjs), storeObjs)
	}
	ptr, err := storageObjToPersonaTagStorageObj(storeObjs[0])
	if err != nil {
		return nil, err
	}
	return ptr, nil
}

// setPersonaTagStorageObj saves the given personaTagStorageObj to the Nakama DB for the current user.
func setPersonaTagStorageObj(ctx context.Context, nk runtime.NakamaModule, obj *personaTagStorageObj) error {
	userID, err := getUserID(ctx)
	if err != nil {
		return fmt.Errorf("unable to get user ID: %w", err)
	}
	buf, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("unable to marshal persona tag storage object: %w", err)
	}
	write := &runtime.StorageWrite{
		Collection:      cardinalCollection,
		Key:             personaTagKey,
		UserID:          userID,
		Value:           string(buf),
		PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
		PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
	}

	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{write})
	if err != nil {
		return err
	}
	return nil
}

// handleClaimPersona handles a request to Nakama to associate the current user with the persona tag in the payload.
func handleClaimPersona(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	if ptr, err := getPersonaTagStorageObj(ctx, nk); err != nil && err != ErrorPersonaTagStorageObjNotFound {
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
	if err := setPersonaTagStorageObj(ctx, nk, ptr); err != nil {
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
		if err := setPersonaTagStorageObj(ctx, nk, ptr); err != nil {
			return logError(logger, "unable to set persona tag storage object: %w", err)
		}
		return logCode(logger, ALREADY_EXISTS, "persona tag %q is not available", ptr.PersonaTag)
	}

	tick, err := cardinalCreatePersona(ctx, nk, ptr.PersonaTag)
	if err != nil {
		return logError(logger, "unable to make create persona request to cardinal: %w", err)
	}

	ptr.Tick = tick
	if err := setPersonaTagStorageObj(ctx, nk, ptr); err != nil {
		return logError(logger, "unable to save persona tag storage object: %w", err)
	}
	return ptr.toJSON()
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
	ptr, err := getPersonaTagStorageObj(ctx, nk)
	if errors.Is(err, ErrorPersonaTagStorageObjNotFound) {
		return logError(logger, "no persona tag found: %w", err)
	} else if err != nil {
		return logError(logger, "unable to get persona tag storage object: %w", err)
	}

	if ptr.Status == personaTagStatusPending {
		logger.Debug("persona tag status is pending. Attempting to verify against cardinal.")
		verified, err := verifyPersonaTag(ctx, ptr)
		if err == ErrorPersonaSignerUnknown {
			// The status should remain pending
			return ptr.toJSON()
		} else if err != nil {
			return logError(logger, "signer address could not be verified: %w", err)
		}
		logger.Debug("done with request. verified is %v", verified)
		if verified {
			ptr.Status = personaTagStatusAccepted
		} else {
			ptr.Status = personaTagStatusRejected
		}
		if err := setPersonaTagStorageObj(ctx, nk, ptr); err != nil {
			return logError(logger, "unable to set persona tag storage object: %w", err)
		}
	}
	return ptr.toJSON()
}

// initCardinalEndpoints queries the cardinal server to find the list of existing endpoints, and attempts to
// set up RPC wrappers around each one.
func initCardinalEndpoints(logger runtime.Logger, initializer runtime.Initializer) error {
	endpoints, err := cardinalListAllEndpoints()
	if err != nil {
		return fmt.Errorf("failed to get list of cardinal endpoints: %w", err)
	}

	for _, e := range endpoints {
		logger.Debug("registering: %v", e)
		currEndpoint := e
		if currEndpoint[0] == '/' {
			currEndpoint = currEndpoint[1:]
		}
		err := initializer.RegisterRpc(currEndpoint, func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
			logger.Debug("Got request for %q", currEndpoint)

			signedPayload, err := makeSignedPayload(ctx, nk, payload)
			if err != nil {
				return logError(logger, "unable to make signed payload: %v", err)
			}

			req, err := http.NewRequestWithContext(ctx, "POST", makeURL(currEndpoint), signedPayload)
			if err != nil {
				return logError(logger, "request setup failed for endpoint %q: %v", currEndpoint, err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return logError(logger, "request failed for endpoint %q: %v", currEndpoint, err)
			}
			if resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				return logError(logger, "bad status code: %v: %s", resp.Status, body)
			}
			str, err := io.ReadAll(resp.Body)
			if err != nil {
				return logError(logger, "can't read body: %v", err)
			}
			return string(str), nil
		})
		if err != nil {
			return err
		}
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
	personaTag, err := getPersonaTag(ctx, nk)
	if err != nil {
		return nil, err
	}

	pk, nonce, err := getPrivateKeyAndANonce(ctx, nk)
	sp, err := sign.NewSignedString(pk, personaTag, globalNamespace, nonce, payload)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(sp)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(buf), nil
}

func (p *personaTagStorageObj) toJSON() (string, error) {
	buf, err := json.Marshal(p)
	return string(buf), err
}
