package main

import (
	"context"
	"database/sql"
	"encoding/json"
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

/*
	STORAGE MODEL
*/

type Save struct {
	Data     string `json:"data"`
	Persona  string `json:"persona"`
	Verified bool   `json:"verified"`
}

const (
	gameSaveCollection = "game_saves"
)

func initSaveFileStorage(_ runtime.Logger, initializer runtime.Initializer) error {
	err := initializer.RegisterRpc(
		"save",
		handleSaveGame,
	)
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}

func handleSaveGame(ctx context.Context, logger runtime.Logger, _ *sql.DB, nk runtime.NakamaModule, payload string,
) (string, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return logErrorMessageFailedPrecondition(logger, eris.Wrap(err, ""), "failed to get user ID")
	}

	var msg SaveGameRequest
	err = json.Unmarshal([]byte(payload), &msg)
	if err != nil {
		return logError(
			logger,
			eris.Wrap(err, `error unmarshalling payload: expected form {"data": <string>}`),
			InvalidArgument)
	}
	// do not allow empty requests
	if msg.Data == "" {
		return logErrorFailedPrecondition(
			logger,
			eris.New("data cannot be empty"),
		)
	}

	// get the persona storage object.
	persona, err := loadPersonaTagStorageObj(ctx, nk)
	if err != nil {
		if eris.Is(eris.Cause(err), ErrPersonaTagStorageObjNotFound) {
			return "", eris.Wrap(err, "cannot save game: user has not yet claimed a persona tag")
		}
		return logErrorFailedPrecondition(logger, eris.Wrap(err, "failed to get persona for save"))
	}
	// do not allow saving if they do not yet have an accepted tag.
	if persona.Status != personaTagStatusAccepted {
		return logErrorFailedPrecondition(
			logger,
			eris.Errorf("persona tag %q is not yet verified: cannot save", persona.PersonaTag),
		)
	}

	// get allowlist info
	var verified bool
	err = checkVerified(ctx, nk, userID)
	if err != nil {
		// as long as the error isnt that they're not allowlisted, return the error
		if !eris.Is(eris.Cause(err), ErrNotAllowlisted) {
			return logErrorFailedPrecondition(logger, eris.Wrap(err, "could not read verification table"))
		}
	} else {
		// when err == nil, that means checkVerified passed, or that there is no allowlist enabled.
		// so we just set verified to true.
		verified = true
	}

	save := Save{
		Data:     msg.Data,
		Persona:  persona.PersonaTag,
		Verified: verified,
	}
	saveBz, err := json.Marshal(save)
	if err != nil {
		return logErrorFailedPrecondition(logger, eris.Wrap(err, "failed to marshal save file"))
	}

	err = writeSave(ctx, userID, string(saveBz), nk)
	if err != nil {
		return logErrorFailedPrecondition(
			logger,
			eris.Wrap(err, "failed to write game save to storage"),
		)
	}

	response, err := json.Marshal(SaveGameResponse{Success: true})
	if err != nil {
		return logErrorFailedPrecondition(logger, eris.Wrap(err, "failed to marshal response"))
	}

	return string(response), nil
}

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
		"get-save",
		func(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string,
		) (string, error) {
			userID, err := getUserID(ctx)
			if err != nil {
				return logErrorMessageFailedPrecondition(logger, eris.Wrap(err, ""), "failed to get user ID")
			}

			save, err := readSave(ctx, userID, nk)
			if err != nil {
				return logErrorMessageFailedPrecondition(
					logger,
					eris.Wrap(err, "failed to read save"),
					"failed to read save for user %s", userID,
				)
			}
			return save, nil
		},
	)
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}

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
		return "", eris.New("save data not found")
	}
	if len(saves) != 1 {
		return "", eris.Errorf("expected 1 save file, got %d", len(saves))
	}
	return saves[0].Value, nil
}
