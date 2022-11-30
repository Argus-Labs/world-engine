package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
)

// InitializerFunction contains the function signature (minus function name, which MUST be InitModule) that the nakama runtime expects.
type InitializerFunction func(context.Context, runtime.Logger, *sql.DB, runtime.NakamaModule, runtime.Initializer) error

// defining our init function. you can edit here to add to the behavior of the application.
var moduleInit InitializerFunction = func(ctx context.Context, logger runtime.Logger, db *sql.DB, module runtime.NakamaModule, initializer runtime.Initializer) error {
	initStart := time.Now()

	if err := initializer.RegisterRpc("health", RpcHealthCheck); err != nil {
		return err
	}

	logger.Info("module loaded in %vms", time.Since(initStart).Milliseconds())
	return nil
}

// InitModule initializes the module. The Nakama runtime loads up the shared object file, and looks for a function named
// "InitModule" with the same signature as below. Do not edit any of the params/return type, or add any additional params/return types.
// Doing so will break the Nakama runtime from initializing our SO file.
func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, module runtime.NakamaModule, initializer runtime.Initializer) error {
	return moduleInit(ctx, logger, db, module, initializer)
}

/*
	custom rpc stuff
*/

type HealthCheckResponse struct {
	Success  bool   `json:"success"`
	Response string `json:"response"`
}

func RpcHealthCheck(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
	logger.Debug("Healthcheck RPC called")
	response := &HealthCheckResponse{Success: true}
	out, err := json.Marshal(response)
	if err != nil {
		logger.Error("cannot marshal response: %w", err)
		return "", runtime.NewError("cannot marshal response", 13)
	}
	return string(out), nil
}
