package keeper

import (
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"context"
	"google.golang.org/protobuf/proto"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

var _ types.MsgServer = &Keeper{}

func (k *Keeper) SubmitCardinalTx(ctx context.Context, msg *types.SubmitCardinalTxRequest) (*types.SubmitCardinalTxResponse, error) {
	if msg.Sender != k.auth {
		return nil, sdkerrors.ErrUnauthorized.Wrap("SubmitCardinalTx is a system function and cannot be called " +
			"externally.")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sp := new(shardv1.SignedPayload)
	err := proto.Unmarshal(msg.SignedPayload, sp)
	if err != nil {
		return nil, err
	}
	err = k.saveTransaction(sdkCtx, sp.Namespace, msg.SignedPayload)
	if err != nil {
		return nil, err
	}

	return &types.SubmitCardinalTxResponse{}, nil
}
