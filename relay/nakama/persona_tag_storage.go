package main

import (
	"context"
	"encoding/json"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

// personaTagStorageObj contains persona tag information for a specific user, and keeps track of whether the
// persona tag has been successfully registered with cardinal.
type personaTagStorageObj struct {
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
	personaTagStatusPending  personaTagStatus = "pending"
	personaTagStatusAccepted personaTagStatus = "accepted"
	personaTagStatusRejected personaTagStatus = "rejected"
)

// loadPersonaTagStorageObj loads the current user's persona tag storage object from Nakama's storage layer. The
// "current user" comes from the user ID stored in the given context.
func loadPersonaTagStorageObj(ctx context.Context, nk runtime.NakamaModule) (*personaTagStorageObj, error) {
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
		return nil, eris.Wrap(err, "")
	}
	if len(storeObjs) == 0 {
		return nil, eris.Wrap(ErrPersonaTagStorageObjNotFound, "")
	} else if len(storeObjs) > 1 {
		return nil, eris.Errorf("expected 1 storage object, got %d with values %v", len(storeObjs), storeObjs)
	}
	ptr, err := storageObjToPersonaTagStorageObj(storeObjs[0])
	if err != nil {
		return nil, err
	}
	return ptr, nil
}

// storageObjToPersonaTagStorageObj converts a generic Nakama StorageObject to a locally defined personaTagStorageObj.
func storageObjToPersonaTagStorageObj(obj *api.StorageObject) (*personaTagStorageObj, error) {
	var ptr personaTagStorageObj
	if err := json.Unmarshal([]byte(obj.Value), &ptr); err != nil {
		return nil, eris.Wrap(err, "unable to unmarshal persona tag storage obj")
	}
	ptr.version = obj.Version
	return &ptr, nil
}

// attemptToUpdatePending attempts to change the given personaTagStorageObj's Status from "pending" to either "accepted"
// or "rejected" by using cardinal as the source of truth. If the Status is not "pending", this call is a no-op.
func (p *personaTagStorageObj) attemptToUpdatePending(ctx context.Context, nk runtime.NakamaModule,
) (*personaTagStorageObj, error) {
	if p.Status != personaTagStatusPending {
		return p, nil
	}

	verified, err := p.verifyPersonaTag(ctx)
	if eris.Is(eris.Cause(err), ErrPersonaSignerUnknown) {
		// Leave the Status as pending.
		return p, nil
	} else if err != nil {
		return nil, err
	}
	if verified {
		p.Status = personaTagStatusAccepted
	} else {
		p.Status = personaTagStatusRejected
	}
	// Attempt to save the updated Status to Nakama. One reason this can fail is that the underlying record was
	// updated while this processing was going on. Whatever the reason, re-fetch this record from Nakama's storage.
	if err = p.savePersonaTagStorageObj(ctx, nk); err != nil {
		return loadPersonaTagStorageObj(ctx, nk)
	}
	return p, nil
}

// verifyPersonaTag queries cardinal to see if the signer address for the given persona tag matches Nakama's signer
// address.
func (p *personaTagStorageObj) verifyPersonaTag(ctx context.Context) (verified bool, err error) {
	gameSignerAddress, err := cardinalQueryPersonaSigner(ctx, p.PersonaTag, p.Tick)
	if err != nil {
		return false, err
	}
	nakamaSignerAddress := getSignerAddress()
	return gameSignerAddress == nakamaSignerAddress, nil
}

// savePersonaTagStorageObj saves the given personaTagStorageObj to the Nakama DB for the current user.
func (p *personaTagStorageObj) savePersonaTagStorageObj(ctx context.Context, nk runtime.NakamaModule) error {
	userID, err := getUserID(ctx)
	if err != nil {
		return eris.Wrap(err, "unable to get user ID")
	}
	buf, err := json.Marshal(p)
	if err != nil {
		return eris.Wrap(err, "unable to marshal persona tag storage object")
	}
	write := &runtime.StorageWrite{
		Collection:      cardinalCollection,
		Key:             personaTagKey,
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

func (p *personaTagStorageObj) toJSON() (string, error) {
	buf, err := json.Marshal(p)
	return string(buf), eris.Wrap(err, "")
}
