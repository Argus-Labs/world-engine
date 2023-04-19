package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	// SIDECAR
	g1 "buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
	// CARDINAL
	"buf.build/gen/go/argus-labs/cardinal/grpc/go/ecs/ecsv1grpc"
)

var (
	sidecar  g1.SidecarClient     = nil
	cardinal ecsv1grpc.GameClient = nil

	CustomRPCs = []CustomRPC{StartGameRPC, MintCoinsRPC}
)

// InitializerFunction contains the function signature (minus function name, which MUST be InitModule) that the nakama runtime expects.
type InitializerFunction func(context.Context, runtime.Logger, *sql.DB, runtime.NakamaModule, runtime.Initializer) error

// defining our init function. you can edit here to add to the behavior of the application.
var moduleInit InitializerFunction = func(ctx context.Context, logger runtime.Logger, db *sql.DB, module runtime.NakamaModule, initializer runtime.Initializer) error {
	initStart := time.Now()

	for _, c := range CustomRPCs {
		if err := initializer.RegisterRpc(c.name, c.f); err != nil {
			return err
		}
	}

	logger.Info("module loaded in %vms", time.Since(initStart).Milliseconds())
	return nil
}

// InitModule initializes the module. The Nakama runtime loads up the shared object file, and looks for a function named
// "InitModule" with the same signature as below. Do not edit any of the params/return type, or add any additional params/return types.
// Doing so will break the Nakama runtime from initializing our SO file.
func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, module runtime.NakamaModule, initializer runtime.Initializer) error {
	cfg := LoadConfig()
	var err error
	sidecar, err = getClient[g1.SidecarClient](cfg.SidecarTarget, g1.NewSidecarClient)
	if err != nil {
		return err
	}
	cardinal, err = getClient[ecsv1grpc.GameClient](cfg.CardinalTarget, ecsv1grpc.NewGameClient)
	if err != nil {
		return err
	}

	return moduleInit(ctx, logger, db, module, initializer)
}

func getClient[client any](target string, getter func(grpc.ClientConnInterface) client) (client, error) {
	clientConn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	grpcClient := getter(clientConn)
	return grpcClient, nil
}

/*
	custom rpc stuff
*/
