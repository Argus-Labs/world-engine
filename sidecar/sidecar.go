package sidecar

import (
	"context"
	"fmt"
	"net"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	types2 "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/tendermint/tendermint/libs/log"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	"google.golang.org/grpc"

	g1 "buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"

	v1 "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"

	"github.com/argus-labs/argus/pool"
	adaptertypesv1 "github.com/argus-labs/argus/x/adapter/types/v1"
)

const (
	ModuleName = "sidecar"
)

type Sidecar struct {
	rtr               *baseapp.MsgServiceRouter
	qry               *baseapp.GRPCQueryRouter
	pool              pool.MsgPoolSender
	cms               types.CommitMultiStore
	bk                bankkeeper.Keeper
	adapterModuleAddr string
	logger            log.Logger
}

func (s Sidecar) UpdateGameState(ctx context.Context, state *v1.MsgUpdateGameState) (*v1.MsgUpdateGameStateResponse, error) {
	msg := &adaptertypesv1.MsgUpdateGameState{Sender: s.adapterModuleAddr, NumPlanets: state.NumPlanets}
	s.pool.Send(msg)
	return &v1.MsgUpdateGameStateResponse{}, nil
}

func (s Sidecar) EthTx(ctx context.Context, tx *v1.MsgEthTx) (*v1.MsgEthTxResponse, error) {
	//msg := evmTypes.MsgEthereumTx{
	//	Data:  tx.Data,
	//	Size_: 0,
	//	Hash:  "",
	//	From:  "",
	//}
	return nil, nil
}

// StartSidecar opens the gRPC server.
func StartSidecar(rtr *baseapp.MsgServiceRouter, qry *baseapp.GRPCQueryRouter, bk bankkeeper.Keeper, cms types.CommitMultiStore, logger log.Logger, pool pool.MsgPoolSender, adapterModuleAddr string) error {
	sc := Sidecar{rtr: rtr, qry: qry, bk: bk, cms: cms, logger: logger, pool: pool, adapterModuleAddr: adapterModuleAddr}
	port := 5050
	lis, err := net.Listen("tcp", fmt.Sprintf("node:%d", port))
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	g1.RegisterSidecarServer(grpcServer, sc)
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

var _ g1.SidecarServer = Sidecar{}

func (s Sidecar) Ping(ctx context.Context, ping *v1.MsgPing) (*v1.MsgPingResponse, error) {
	return &v1.MsgPingResponse{Id: "pong!"}, nil
}

func (s Sidecar) MintCoins(ctx context.Context, msg *v1.MsgMintCoins) (*v1.MsgMintCoinsResponse, error) {
	sdkCtx := s.getSDKCtx().WithContext(ctx)
	err := s.bk.MintCoins(sdkCtx, ModuleName, types.Coins{types.NewInt64Coin(msg.Denom, msg.Amount)})
	if err != nil {
		return nil, err
	}
	return &v1.MsgMintCoinsResponse{}, nil
}

func (s Sidecar) SendCoins(ctx context.Context, msg *v1.MsgSendCoins) (*v1.MsgSendCoinsResponse, error) {
	msgSend := types2.MsgSend{
		FromAddress: msg.Sender,
		ToAddress:   msg.Recipient,
		Amount:      types.Coins{types.NewInt64Coin(msg.Denom, int64(msg.Amount))},
	}
	s.pool.Send(&msgSend)
	return &v1.MsgSendCoinsResponse{}, nil
}
