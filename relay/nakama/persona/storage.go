package persona

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/utils"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

// StorageObj contains persona tag information for a specific user, and keeps track of whether the
// persona tag has been successfully registered with cardinal.
type StorageObj struct {
	PersonaTag string           `json:"personaTag"`
	Status     personaTagStatus `json:"status"`
	Tick       uint64           `json:"tick"`
	TxHash     string           `json:"txHash"`
	// version is used with Nakama storage layer to allow for optimistic locking. Saving this storage
	// object succeeds only if the passed in version matches the version in the storage layer.
	// see https://heroiclabs.com/docs/nakama/concepts/storage/collections/#conditional-writes for more info.
	version string `json:"-"`
}

type personaTagStatus string

const (
	StatusPending      personaTagStatus = "pending"
	StatusAccepted     personaTagStatus = "accepted"
	StatusRejected     personaTagStatus = "rejected"
	PersonaTagKey                       = "personaTag"
	CardinalCollection                  = "cardinalCollection"
)

// LoadPersonaTagStorageObj loads the current user's persona tag storage object from Nakama's storage layer. The
// "current user" comes from the user ID stored in the given context.
func LoadPersonaTagStorageObj(ctx context.Context, nk runtime.NakamaModule) (*StorageObj, error) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	storeObjs, err := nk.StorageRead(ctx, []*runtime.StorageRead{
		{
			Collection: CardinalCollection,
			Key:        PersonaTagKey,
			UserID:     userID,
		},
	})
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	if len(storeObjs) == 0 {
		return nil, eris.Wrap(ErrPersonaTagStorageObjNotFound, "")
	} else if len(storeObjs) > 1 {
		return nil, eris.Errorf("expected 1 storage object, got %d with values %v", len(storeObjs), storeObjs)
	}
	ptr, err := StorageObjToPersonaTagStorageObj(storeObjs[0])
	if err != nil {
		return nil, err
	}
	return ptr, nil
}

// StorageObjToPersonaTagStorageObj converts a generic Nakama StorageObject to a locally defined StorageObj.
func StorageObjToPersonaTagStorageObj(obj *api.StorageObject) (*StorageObj, error) {
	var ptr StorageObj
	if err := json.Unmarshal([]byte(obj.Value), &ptr); err != nil {
		return nil, eris.Wrap(err, "unable to unmarshal persona tag storage obj")
	}
	ptr.version = obj.Version
	return &ptr, nil
}

// AttemptToUpdatePending attempts to change the given StorageObj's Status from "pending" to either "accepted"
// or "rejected" by using cardinal as the source of truth. If the Status is not "pending", this call is a no-op.
func (p *StorageObj) AttemptToUpdatePending(
	ctx context.Context,
	nk runtime.NakamaModule,
	cardinalAddr string,
) (*StorageObj, error) {
	if p.Status != StatusPending {
		return p, nil
	}

	verified, err := p.verifyPersonaTag(ctx, cardinalAddr)
	switch {
	case eris.Is(eris.Cause(err), ErrPersonaSignerUnknown):
		// Leave the Status as pending.
		return p, nil
	case eris.Is(eris.Cause(err), ErrPersonaSignerAvailable):
		// Somehow Nakama thinks this persona tag belongs to this user, but Cardinal doesn't think the persona tag
		// belongs to anyone. Just reject this on Nakama's end so the user can try a different persona tag.
		// Incidentally, trying the same persona tag might work.
		p.Status = StatusRejected
	case err != nil:
		return nil, eris.Wrap(err, "error when verifying persona tag; user may be stuck in pending")
	default:
		if verified {
			p.Status = StatusAccepted
		} else {
			p.Status = StatusRejected
		}
	}
	// Attempt to save the updated Status to Nakama. One reason this can fail is that the underlying record was
	// updated while this processing was going on. Whatever the reason, re-fetch this record from Nakama's storage.
	if err = p.SavePersonaTagStorageObj(ctx, nk); err != nil {
		return LoadPersonaTagStorageObj(ctx, nk)
	}
	return p, nil
}

// verifyPersonaTag queries cardinal to see if the signer address for the given persona tag matches Nakama's signer
// address.
func (p *StorageObj) verifyPersonaTag(ctx context.Context, cardinalAddr string) (verified bool, err error) {
	gameSignerAddress, err := queryPersonaSigner(ctx, p.PersonaTag, p.Tick, cardinalAddr)
	if err != nil {
		return false, err
	}
	nakamaSignerAddress := signer.GetSignerAddress()
	return gameSignerAddress == nakamaSignerAddress, nil
}

// SavePersonaTagStorageObj saves the given StorageObj to the Nakama DB for the current user.
func (p *StorageObj) SavePersonaTagStorageObj(ctx context.Context, nk runtime.NakamaModule) error {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return eris.Wrap(err, "unable to get user ID")
	}
	buf, err := json.Marshal(p)
	if err != nil {
		return eris.Wrap(err, "unable to marshal persona tag storage object")
	}
	write := &runtime.StorageWrite{
		Collection:      CardinalCollection,
		Key:             PersonaTagKey,
		UserID:          userID,
		Value:           string(buf),
		Version:         p.version,
		PermissionRead:  runtime.STORAGE_PERMISSION_NO_READ,
		PermissionWrite: runtime.STORAGE_PERMISSION_NO_WRITE,
	}

	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{write})
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}

func queryPersonaSigner(
	ctx context.Context,
	personaTag string,
	tick uint64,
	cardinalAddr string,
) (signerAddress string, err error) {
	readPersonaRequest := struct {
		PersonaTag string `json:"personaTag"`
		Tick       uint64 `json:"tick"`
	}{
		PersonaTag: personaTag,
		Tick:       tick,
	}

	buf, err := json.Marshal(readPersonaRequest)
	if err != nil {
		return "", eris.Wrap(err, "")
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		utils.MakeHTTPURL(readPersonaSignerEndpoint, cardinalAddr),
		bytes.NewReader(buf),
	)
	if err != nil {
		return "", eris.Wrap(err, "")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := utils.DoRequest(httpReq)
	if err != nil {
		return "", err
	}
	defer httpResp.Body.Close()

	var resp struct {
		Status        string `json:"status"`
		SignerAddress string `json:"signerAddress"`
	}
	if err = json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return "", eris.Wrap(err, "")
	}
	if resp.Status == readPersonaSignerStatusUnknown {
		return "", eris.Wrap(ErrPersonaSignerUnknown, "")
	} else if resp.Status == readPersonaSignerStatusAvailable {
		return "", eris.Wrap(ErrPersonaSignerAvailable, "")
	}
	return resp.SignerAddress, nil
}

func (p *StorageObj) ToJSON() (string, error) {
	buf, err := json.Marshal(p)
	return string(buf), eris.Wrap(err, "")
}
