package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"

	"buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
	sidecarv1 "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"
	"github.com/heroiclabs/nakama-common/runtime"
	"google.golang.org/grpc"
)

var _ sidecarv1grpc.NakamaServer = &CosmosReceiver{}

type CosmosReceiver struct {
	db     *sql.DB
	logger runtime.Logger
	nm     runtime.NakamaModule
	port   uint64
}

func NewCosmosReceiver(db *sql.DB, logger runtime.Logger, nm runtime.NakamaModule, port uint64) CosmosReceiver {
	return CosmosReceiver{db, logger, nm, port}
}

func (c *CosmosReceiver) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", c.port))
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	sidecarv1grpc.RegisterNakamaServer(grpcServer, c)
	go func() {
		err := grpcServer.Serve(lis)
		if err != nil {
			c.logger.Error("grpc server error", "error", err.Error())
		}
	}()
	return nil
}

func (c CosmosReceiver) CompleteQuest(ctx context.Context, quest *sidecarv1.MsgCompleteQuest) (*sidecarv1.MsgCompleteQuestResponse, error) {
	c.logger.Info("QUEST COMPLETE!")
	return &sidecarv1.MsgCompleteQuestResponse{Success: true}, nil
}
