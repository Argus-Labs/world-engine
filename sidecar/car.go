package sidecar

import (
	"context"
	"fmt"
	"net"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/tendermint/tendermint/libs/log"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	"google.golang.org/grpc"

	v1 "github.com/argus-labs/argus/sidecar/v1"
)

const (
	moduleName = "sidecar"
)

type Sidecar struct {
	rtr    *baseapp.MsgServiceRouter
	qry    *baseapp.GRPCQueryRouter
	cms    types.CommitMultiStore
	bk     bankkeeper.Keeper
	logger log.Logger
}

// StartSidecar opens the gRPC server.
func StartSidecar(rtr *baseapp.MsgServiceRouter, qry *baseapp.GRPCQueryRouter, bk bankkeeper.Keeper, cms types.CommitMultiStore, logger log.Logger) error {
	sc := Sidecar{rtr: rtr, qry: qry, bk: bk, cms: cms, logger: logger}
	port := 5050
	lis, err := net.Listen("tcp", fmt.Sprintf("node:%d", port))
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	v1.RegisterSidecarServer(grpcServer, sc)
	go func() {
		err := grpcServer.Serve(lis)
		if err != nil {
			logger.Error("grpc server error", "error", err.Error())
		}
	}()
	return nil
}

func (s Sidecar) getSDKCtx() types.Context {
	return types.NewContext(s.cms, tmproto.Header{}, false, s.logger)
}

var _ v1.SidecarServer = Sidecar{}

func (s Sidecar) Ping(ctx context.Context, ping *v1.MsgPing) (*v1.MsgPingResponse, error) {
	return &v1.MsgPingResponse{Id: "pong!"}, nil
}

func (s Sidecar) MintCoins(ctx context.Context, msg *v1.MsgMintCoins) (*v1.MsgMintCoinsResponse, error) {
	sdkCtx := s.getSDKCtx().WithContext(ctx)
	err := s.bk.MintCoins(sdkCtx, moduleName, types.Coins{types.NewInt64Coin(msg.Denom, msg.Amount)})
	if err != nil {
		return nil, err
	}
	return &v1.MsgMintCoinsResponse{}, nil
}
