package main

import (
	"context"
	"database/sql"

	ecsv1 "buf.build/gen/go/argus-labs/cardinal/protocolbuffers/go/ecs"
	"github.com/golang/protobuf/jsonpb"
	"github.com/heroiclabs/nakama-common/runtime"
)

var (
	StartGameRPC = NewCustomRPC("start-game", func(ctx context.Context, logger runtime.Logger, db *sql.DB, module runtime.NakamaModule, payload string) (string, error) {
		res, err := cardinal.StartGameLoop(ctx, &ecsv1.MsgStartGameLoop{})
		if err != nil {
			return "", err
		}

		m := &jsonpb.Marshaler{}
		return m.MarshalToString(res)
	})
)
