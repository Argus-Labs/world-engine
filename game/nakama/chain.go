package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	v1 "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"
	"github.com/heroiclabs/nakama-common/runtime"
)

type MintCoinsResponse struct {
	Success  bool   `json:"success"`
	Response string `json:"response"`
}

var MintCoinsRPC = NewCustomRPC("mint-coins", func(ctx context.Context, logger runtime.Logger, db *sql.DB, module runtime.NakamaModule, payload string) (string, error) {
	logger.Debug("MintCoins RPC called")
	response := &MintCoinsResponse{Success: true}
	res, err := sidecar.MintCoins(ctx, &v1.MsgMintCoins{Amount: 10, Denom: "NAKAMA"})
	if err != nil {
		return "", runtime.NewError(fmt.Sprintf("call to sidecar failed: %s", err.Error()), 1)
	}
	logger.Info("mint coins response: %s", res.String())
	response.Response = res.String()
	out, err := json.Marshal(response)
	if err != nil {
		logger.Error("cannot marshal response: %w", err)
		return "", runtime.NewError("cannot marshal response", 13)
	}
	return string(out), nil
})
