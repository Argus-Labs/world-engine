package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	nakamaerrors "pkg.world.dev/world-engine/relay/nakama/errors"
	"pkg.world.dev/world-engine/relay/nakama/persona"
	"pkg.world.dev/world-engine/relay/nakama/utils"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

/*
	REQUEST MESSAGES
*/

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

const (
	gameSaveCollection = "game_saves"
)

func writeSave(ctx context.Context, userID string, save string, nk runtime.NakamaModule) error {
	write := &runtime.StorageWrite{
		Collection:      gameSaveCollection,
		Key:             userID,
		UserID:          userID,
		Value:           save,
		Version:         "",
		PermissionRead:  runtime.STORAGE_PERMISSION_OWNER_READ,
		PermissionWrite: runtime.STORAGE_PERMISSION_OWNER_WRITE,
	}
	_, err := nk.StorageWrite(ctx, []*runtime.StorageWrite{write})
	return err
}

func initSaveFileQuery(_ runtime.Logger, initializer runtime.Initializer) error {
	err := initializer.RegisterRpc(
		"nakama/get-save",
		handleGetSaveGame,
	)
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}

func handleGetSaveGame(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, _ string,
) (string, error) {
	userID, err := utils.GetUserID(ctx)
	if err != nil {
		return utils.LogErrorMessageFailedPrecondition(logger, eris.Wrap(err, ""), "failed to get user ID")
	}

	var personaTag string
	// get the persona storage object.
	p, err := persona.LoadPersonaTagStorageObj(ctx, nk)
	if err != nil {
		// we ignore the error where the tag is not found.
		// all other errors should be returned.
		if !eris.Is(eris.Cause(err), nakamaerrors.ErrPersonaTagStorageObjNotFound) {
			return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "failed to get persona for save"))
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
		return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "could not read verification table"))
	}

	var dataStr string
	data, err := readSave(ctx, userID, nk)
	if err != nil {
		// if no save is found, we just wanna return the empty string. so catch all other errors but that one.
		if !eris.Is(eris.Cause(err), ErrNoSaveFound) {
			return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "failed to read save data"))
		}
	} else {
		var dataMsg SaveGameRequest
		err := json.Unmarshal([]byte(data), &dataMsg)
		if err != nil {
			return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "failed to unmarshall save"))
		}
		dataStr = dataMsg.Data
	}

	saveData := GetSaveReply{
		Data:        dataStr,
		Persona:     personaTag,
		Allowlisted: verified,
	}
	saveBz, err := json.Marshal(saveData)
	if err != nil {
		return utils.LogErrorFailedPrecondition(logger, eris.Wrap(err, "failed to marshal save file"))
	}
	return string(saveBz), nil
}

var ErrNoSaveFound = errors.New("no save found")

func readSave(ctx context.Context, userID string, nk runtime.NakamaModule) (string, error) {
	read := &runtime.StorageRead{
		Collection: gameSaveCollection,
		Key:        userID,
		UserID:     userID,
	}
	saves, err := nk.StorageRead(ctx, []*runtime.StorageRead{read})
	if err != nil {
		return "", err
	}
	if len(saves) == 0 {
		return "", eris.Wrapf(ErrNoSaveFound, "")
	}
	if len(saves) != 1 {
		return "", eris.Errorf("expected 1 save file, got %d", len(saves))
	}
	return saves[0].Value, nil
}
