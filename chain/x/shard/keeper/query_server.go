package keeper

import (
	"context"
	sdk "github.com/cosmos/cosmos-sdk/types"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

var _ types.QueryServer = &Keeper{}

func (k *Keeper) Transactions(ctx context.Context, req *types.QueryTransactionsRequest,
) (*types.QueryTransactionsResponse, error) {
	if req.Namespace == "" {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("namespace required but not supplied")
	}
	key, limit := types.ExtractPageRequest(req.Page)
	res := types.QueryTransactionsResponse{
		Ticks: make([]*types.Tick, 0, limit),
		Page:  &types.PageResponse{},
	}
	count := uint32(0)
	k.iterateTransactions(sdk.UnwrapSDKContext(ctx), key, nil,
		req.Namespace, func(tick uint64, txs *types.Transactions) bool {
			// we keep the check here so that if we hit the limit,
			// we return the NEXT key in the iteration, not the one before it.
			if count == limit {
				res.Page.Key = k.getTransactionKey(tick)
				return false
			}
			res.Ticks = append(res.Ticks, &types.Tick{
				Tick: tick,
				Txs:  txs,
			})
			return true
		})

	return &res, nil
}
