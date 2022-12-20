package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"

	"github.com/heroiclabs/nakama-common/runtime"
	"google.golang.org/grpc"

	v1 "github.com/argus-labs/argus/nakama/v1"
)

var _ v1.NakamaServer = &CosmosReceiver{}

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
	lis, err := net.Listen("tcp", fmt.Sprintf("nakama:%d", c.port))
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	v1.RegisterNakamaServer(grpcServer, c)
	go func() {
		err := grpcServer.Serve(lis)
		if err != nil {
			c.logger.Error("grpc server error", "error", err.Error())
		}
	}()
	return nil
}

func (c CosmosReceiver) CompleteQuest(ctx context.Context, quest *v1.MsgCompleteQuest) (*v1.MsgCompleteQuestResponse, error) {
	c.logger.Info("QUEST COMPLETE!")
	return &v1.MsgCompleteQuestResponse{Success: true}, nil
}
