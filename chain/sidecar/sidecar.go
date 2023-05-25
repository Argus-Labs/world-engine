package sidecar

import (
	"context"
	"fmt"
	"net"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/sidecar/v1/sidecarv1grpc"
	sidecarv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/sidecar/v1"
	"google.golang.org/grpc"

	"cosmossdk.io/log"
	"cosmossdk.io/store"

	cometTypes "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
)

const (
	ModuleName = "sidecar"
)

type Sidecar struct {
	rtr    *baseapp.MsgServiceRouter
	qry    *baseapp.GRPCQueryRouter
	cms    store.CacheMultiStore
	bk     bankkeeper.Keeper
	logger log.Logger
}

// StartSidecar opens the gRPC server.
func StartSidecar(rtr *baseapp.MsgServiceRouter, qry *baseapp.GRPCQueryRouter,
	bk bankkeeper.Keeper, cms store.CacheMultiStore, logger log.Logger) error {
	sc := Sidecar{rtr: rtr, qry: qry, bk: bk, cms: cms, logger: logger}
	port := 42091
	lis, err := net.Listen("tcp", fmt.Sprintf("node:%d", port))
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	sidecarv1grpc.RegisterSidecarServer(grpcServer, sc)
	go func() {
		localErr := grpcServer.Serve(lis)
		if err != nil {
			logger.Error("grpc server error", "error", localErr.Error())
		}
	}()
	return nil
}

func (s Sidecar) getSDKCtx() types.Context {
	return types.NewContext(s.cms, cometTypes.Header{}, false, s.logger)
}

var _ sidecarv1grpc.SidecarServer = Sidecar{}

func (s Sidecar) MintCoins(ctx context.Context, msg *sidecarv1.MsgMintCoins) (*sidecarv1.MsgMintCoinsResponse, error) {
	sdkCtx := s.getSDKCtx().WithContext(ctx)
	err := s.bk.MintCoins(sdkCtx, ModuleName, types.Coins{types.NewInt64Coin(msg.Denom, msg.Amount)})
	if err != nil {
		return nil, err
	}
	s.cms.Write()
	return &sidecarv1.MsgMintCoinsResponse{}, nil
}

func (s Sidecar) SendCoins(ctx context.Context, msg *sidecarv1.MsgSendCoins) (*sidecarv1.MsgSendCoinsResponse, error) {
	coins := types.Coins{types.NewInt64Coin(msg.Denom, int64(msg.Amount))}
	sender, err := types.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return nil, err
	}
	recip, err := types.AccAddressFromBech32(msg.Recipient)
	if err != nil {
		return nil, err
	}
	sdkCtx := s.getSDKCtx().WithContext(ctx)
	err = s.bk.SendCoins(sdkCtx, sender, recip, coins)
	if err != nil {
		return nil, err
	}
	s.cms.Write()
	return &sidecarv1.MsgSendCoinsResponse{}, nil
}
