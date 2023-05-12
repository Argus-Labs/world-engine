package sidecar

//
//import (
//	"context"
//	"fmt"
//	"net"
//
//	g1 "buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
//	v1 "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"
//	"google.golang.org/grpc"
//
//	"cosmossdk.io/log"
//	"cosmossdk.io/store"
//
//	cometTypes "github.com/cometbft/cometbft/proto/tendermint/types"
//
//	"github.com/cosmos/cosmos-sdk/baseapp"
//	"github.com/cosmos/cosmos-sdk/types"
//	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
//)
//
//const (
//	ModuleName = "sidecar"
//)
//
//type Sidecar struct {
//	rtr    *baseapp.MsgServiceRouter
//	qry    *baseapp.GRPCQueryRouter
//	cms    store.CacheMultiStore
//	bk     bankkeeper.Keeper
//	logger log.Logger
//}
//
//// StartSidecar opens the gRPC server.
//func StartSidecar(rtr *baseapp.MsgServiceRouter, qry *baseapp.GRPCQueryRouter,
//	bk bankkeeper.Keeper, cms store.CacheMultiStore, logger log.Logger) error {
//	sc := Sidecar{rtr: rtr, qry: qry, bk: bk, cms: cms, logger: logger}
//	port := 5050
//	lis, err := net.Listen("tcp", fmt.Sprintf("node:%d", port))
//	if err != nil {
//		return err
//	}
//	grpcServer := grpc.NewServer()
//	g1.RegisterSidecarServer(grpcServer, sc)
//	go func() {
//		localErr := grpcServer.Serve(lis)
//		if err != nil {
//			logger.Error("grpc server error", "error", localErr.Error())
//		}
//	}()
//	return nil
//}
//
//func (s Sidecar) getSDKCtx() types.Context {
//	return types.NewContext(s.cms, cometTypes.Header{}, false, s.logger)
//}
//
//var _ g1.SidecarServer = Sidecar{}
//
//func (s Sidecar) MintCoins(ctx context.Context, msg *v1.MsgMintCoins) (*v1.MsgMintCoinsResponse, error) {
//	sdkCtx := s.getSDKCtx().WithContext(ctx)
//	err := s.bk.MintCoins(sdkCtx, ModuleName, types.Coins{types.NewInt64Coin(msg.Denom, msg.Amount)})
//	if err != nil {
//		return nil, err
//	}
//	s.cms.Write()
//	return &v1.MsgMintCoinsResponse{}, nil
//}
//
//func (s Sidecar) SendCoins(ctx context.Context, msg *v1.MsgSendCoins) (*v1.MsgSendCoinsResponse, error) {
//	coins := types.Coins{types.NewInt64Coin(msg.Denom, int64(msg.Amount))}
//	sender, err := types.AccAddressFromBech32(msg.Sender)
//	if err != nil {
//		return nil, err
//	}
//	recip, err := types.AccAddressFromBech32(msg.Recipient)
//	if err != nil {
//		return nil, err
//	}
//	sdkCtx := s.getSDKCtx().WithContext(ctx)
//	err = s.bk.SendCoins(sdkCtx, sender, recip, coins)
//	if err != nil {
//		return nil, err
//	}
//	s.cms.Write()
//	return &v1.MsgSendCoinsResponse{}, nil
//}
