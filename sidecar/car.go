package sidecar

import (
	"context"
	"fmt"
	"net"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/tendermint/tendermint/libs/log"
	"google.golang.org/grpc"

	v1 "github.com/argus-labs/argus/sidecar/v1"
)

type Sidecar struct {
	rtr    *baseapp.MsgServiceRouter
	qry    *baseapp.GRPCQueryRouter
	logger log.Logger
}

/*
<<<<<<<EXAMPLE CODE SNIPPET ON HOW TO SERVER THE GRPC SERVER>>>>>>>
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
	  log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	...
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterRouteGuideServer(grpcServer, newServer())
	grpcServer.Serve(lis)
*/

// StartSidecar opens the gRPC server.
func StartSidecar(rtr *baseapp.MsgServiceRouter, qry *baseapp.GRPCQueryRouter, logger log.Logger) error {
	sc := Sidecar{rtr: rtr, qry: qry, logger: logger}
	port := 5050
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	v1.RegisterSidecarServer(grpcServer, sc)
	go grpcServer.Serve(lis)
	return nil
}

var _ v1.SidecarServer = Sidecar{}

func (s Sidecar) Ping(ctx context.Context, ping *v1.MsgPing) (*v1.MsgPingResponse, error) {
	return &v1.MsgPingResponse{Id: "pong!"}, nil
}
