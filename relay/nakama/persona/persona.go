package persona

import (
	"bytes"
	"context"
	"encoding/json"
	errors2 "errors"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"io"
	"net/http"
	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"
	"pkg.world.dev/world-engine/sign"
	"sync"
)

var (
	createPersonaEndpoint            = "tx/persona/create-persona"
	readPersonaSignerEndpoint        = "query/persona/signer"
	readPersonaSignerStatusUnknown   = "unknown"
	readPersonaSignerStatusAvailable = "available"

	ErrPersonaTagStorageObjNotFound = errors2.New("persona tag storage object not found")
	ErrNoPersonaTagForUser          = errors2.New("user does not have a verified persona tag")
	ErrPersonaSignerAvailable       = errors2.New("persona signer is available")
	ErrPersonaSignerUnknown         = errors2.New("persona signer is unknown")
	ErrPersonaTagEmpty              = errors2.New("personaTag field was left empty")
)

type TxResponse struct {
	TxHash string `json:"txHash"`
	Tick   uint64 `json:"tick"`
}

func ClaimPersona(
	ctx context.Context,
	nk runtime.NakamaModule,
	verifier *Verifier,
	notifier *receipt.Notifier,
	personaStorageObj *StorageObj,
	globalCardinalAddress string,
	globalNamespace string,
	globalPersonaTagAssignment *sync.Map,
) (res *StorageObj, err error) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return res, eris.Wrap(err, "failed to get userID for claim persona request")
	}

	// Check if the user is verified. This requires them to input a valid beta key.
	if verified, err := allowlist.IsUserVerified(ctx, nk, userID); err != nil {
		return res, eris.Wrap(err, "failed to check if user is validated")
	} else if !verified {
		if verified {
			return res, eris.Wrap(allowlist.ErrNotAllowlisted, "")
		}
	}

	if personaStorageObj.PersonaTag == "" {
		return res, ErrPersonaTagEmpty
	}

	tag, err := LoadPersonaTagStorageObj(ctx, nk)
	//nolint:gocritic // This if-else chain contains a switch, a nested switch would be worse.
	//revive:disable-next-line:empty-block
	if eris.Is(eris.Cause(err), ErrPersonaTagStorageObjNotFound) {
		// This error is fine, if a storage obj is not found it just means claiming hasn't been attempted yet
	} else if err != nil {
		return res, eris.Wrap(err, "unable to get persona tag storage object")
	} else {
		switch tag.Status {
		case StatusPending:
			return res, eris.Errorf("persona tag %q is pending for this account", tag.PersonaTag)
		case StatusAccepted:
			return res, eris.Errorf("persona tag %q already associated with this account", tag.PersonaTag)
		case StatusRejected:
			// if the tag was rejected, don't do anything. let the user try to claim another tag.
		}
	}

	txHash, tick, err := createPersona(ctx, nk, personaStorageObj.PersonaTag, globalCardinalAddress, globalNamespace)
	if err != nil {
		return res, eris.Wrap(err, "unable to make create persona request to cardinal")
	}
	notifier.AddTxHashToPendingNotifications(txHash, userID)

	personaStorageObj.Status = StatusPending
	if err = personaStorageObj.SavePersonaTagStorageObj(ctx, nk); err != nil {
		return res, eris.Wrap(err, "unable to set persona tag storage object")
	}

	// Try to actually assign this personaTag->UserID in the sync map. If this succeeds, Nakama is OK with this
	// user having the persona tag.
	if ok := setPersonaTagAssignment(personaStorageObj.PersonaTag, userID, globalPersonaTagAssignment); !ok {
		personaStorageObj.Status = StatusRejected
		if err = personaStorageObj.SavePersonaTagStorageObj(ctx, nk); err != nil {
			return res, eris.Wrap(err, "unable to set persona tag storage object")
		}
		return res, eris.Errorf("persona tag %q is not available", personaStorageObj.PersonaTag)
	}

	personaStorageObj.Tick = tick
	personaStorageObj.TxHash = txHash
	if err = personaStorageObj.SavePersonaTagStorageObj(ctx, nk); err != nil {
		return res, eris.Wrap(err, "unable to save persona tag storage object")
	}
	verifier.AddPendingPersonaTag(userID, personaStorageObj.TxHash)
	return res, nil
}

func createPersona(
	ctx context.Context,
	nk runtime.NakamaModule,
	personaTag string,
	cardinalAddr string,
	cardinalNamespace string,
) (
	txHash string,
	tick uint64,
	err error,
) {
	defer func() {
		if r := recover(); r != nil {
			txHash = ""
			tick = 0
			err = eris.Errorf("a panic occurred in nakama in the function, cardinalCreatePersona:, %s", r)
		}
	}()

	signerAddress := signer.GetSignerAddress()
	createPersonaTx := struct {
		PersonaTag    string `json:"personaTag"`
		SignerAddress string `json:"signerAddress"`
	}{
		PersonaTag:    personaTag,
		SignerAddress: signerAddress,
	}

	key, nonce, err := signer.GetPrivateKeyAndANonce(ctx, nk)
	if err != nil {
		return "", 0, eris.Wrapf(err, "unable to get the private key or a nonce")
	}

	transaction, err := sign.NewSystemTransaction(key, cardinalNamespace, nonce, createPersonaTx)

	if err != nil {
		return "", 0, eris.Wrapf(err, "unable to create signed payload")
	}

	buf, err := transaction.Marshal()
	if err != nil {
		return "", 0, eris.Wrapf(err, "unable to marshal signed payload")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		utils.MakeHTTPURL(createPersonaEndpoint, cardinalAddr),
		bytes.NewReader(buf),
	)
	if err != nil {
		return "", 0, eris.Wrapf(err, "unable to make request to %q", createPersonaEndpoint)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := utils.DoRequest(req)
	if err != nil {
		return "", 0, err
	}

	defer resp.Body.Close()

	if code := resp.StatusCode; code != http.StatusOK {
		buf, err = io.ReadAll(resp.Body)
		return "", 0, eris.Wrapf(err, "create persona response is not 200. code %v, body: %v", code, string(buf))
	}

	var createPersonaResponse TxResponse

	if err = json.NewDecoder(resp.Body).Decode(&createPersonaResponse); err != nil {
		return "", 0, eris.Wrap(err, "unable to decode response")
	}
	if createPersonaResponse.TxHash == "" {
		return "", 0, eris.Errorf("tx response does not have a tx hash")
	}
	return createPersonaResponse.TxHash, createPersonaResponse.Tick, nil
}

func ShowPersona(
	ctx context.Context,
	nk runtime.NakamaModule,
	globalCardinalAddress string,
) (res *StorageObj, err error) {
	personaStorageObj, err := LoadPersonaTagStorageObj(ctx, nk)
	if err != nil {
		if eris.Is(eris.Cause(err), ErrPersonaTagStorageObjNotFound) {
			return res, err
		}
		return res, eris.Wrap(err, "unable to get persona tag storage object")
	}
	personaStorageObj, err = personaStorageObj.AttemptToUpdatePending(ctx, nk, globalCardinalAddress)
	if err != nil {
		return res, eris.Wrap(err, "unable to update pending state")
	}
	return personaStorageObj, nil
}

// setPersonaTagAssignment attempts to associate a given persona tag with the given user ID, and returns
// true if the attempt was successful or false if it failed. This method is safe for concurrent access.
func setPersonaTagAssignment(personaTag, userID string, globalPersonaTagAssignment *sync.Map) (ok bool) {
	val, loaded := globalPersonaTagAssignment.LoadOrStore(personaTag, userID)
	if !loaded {
		return true
	}
	gotUserID, _ := val.(string)
	return gotUserID == userID
}
