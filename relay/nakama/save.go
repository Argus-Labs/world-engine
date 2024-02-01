package main

import (
	"context"
	"errors"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/utils"
)

/*
	REQUEST MESSAGES
*/

var ErrNoSaveFound = errors.New("no save found")

const (
	gameSaveCollection = "game_saves"
)

type SaveGameRequest struct {
	Data string `json:"data"`
}

type SaveGameResponse struct {
	Success bool `json:"success"`
}

type GetSaveReply struct {
	Data        string `json:"data"`
	Persona     string `json:"persona"`
	Allowlisted bool   `json:"allowlisted"`
}

func writeSave(ctx context.Context, nk runtime.NakamaModule, msg SaveGameRequest) (res SaveGameResponse, err error) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return res, eris.Wrap(err, "failed to get userID")
	}

	if msg.Data == "" {
		return res, eris.New("data cannot be empty")
	}

	write := &runtime.StorageWrite{
		Collection:      gameSaveCollection,
		Key:             userID,
		UserID:          userID,
		Value:           msg.Data,
		Version:         "",
		PermissionRead:  runtime.STORAGE_PERMISSION_OWNER_READ,
		PermissionWrite: runtime.STORAGE_PERMISSION_OWNER_WRITE,
	}
	_, err = nk.StorageWrite(ctx, []*runtime.StorageWrite{write})
	return SaveGameResponse{Success: true}, err
}

func readSave(ctx context.Context, nk runtime.NakamaModule) (res GetSaveReply, err error) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return res, eris.Wrap(err, "failed to get userID")
	}

	var personaTag string
	p, err := persona.LoadPersonaTagStorageObj(ctx, nk)
	if err != nil {
		if !eris.Is(eris.Cause(err), persona.ErrPersonaTagStorageObjNotFound) {
			return res, err
		}
	} else {
		if p.Status == persona.StatusAccepted {
			personaTag = p.PersonaTag
		}
	}

	// check if the user is allowlisted. NOTE: checkVerified will return true in two cases:
	// case 1: if the allowlist is disabled (via ENABLE_ALLOWLIST env var).
	// case 2: the user is actually allowlisted.
	verified, err := allowlist.IsUserVerified(ctx, nk, userID)
	if err != nil {
		return res, err
	}

	read := &runtime.StorageRead{
		Collection: gameSaveCollection,
		Key:        userID,
		UserID:     userID,
	}
	saves, err := nk.StorageRead(ctx, []*runtime.StorageRead{read})
	if err != nil {
		return res, err
	}
	if len(saves) == 0 {
		return res, eris.Wrapf(ErrNoSaveFound, "")
	}
	if len(saves) != 1 {
		return res, eris.Errorf("expected 1 save file, got %d", len(saves))
	}
	return GetSaveReply{
		Persona:     personaTag,
		Allowlisted: verified,
		Data:        saves[0].Value,
	}, nil
}
