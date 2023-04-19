package main

import (
	"context"
	"database/sql"

	"github.com/heroiclabs/nakama-common/runtime"
)

type RPCFunction func(ctx context.Context, logger runtime.Logger, db *sql.DB, module runtime.NakamaModule, payload string) (string, error)

type CustomRPC struct {
	name string
	f    RPCFunction
}

func NewCustomRPC(name string, f RPCFunction) CustomRPC {
	return CustomRPC{name, f}
}
