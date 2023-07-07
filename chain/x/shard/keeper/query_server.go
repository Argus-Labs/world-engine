package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

var _ types.QueryServer = &Keeper{}

func (k *Keeper) Batches(ctx context.Context, req *types.QueryBatchesRequest) (*types.QueryBatchesResponse, error) {
	if req.Namespace == "" {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("namespace required but not supplied")
	}
	key, limit := types.ExtractPageRequest(req.Page)
	res := types.QueryBatchesResponse{
		Batches: make([]*types.TransactionBatch, 0, limit),
		Page:    &types.PageResponse{},
	}
	count := uint32(0)
	k.iterateBatches(sdk.UnwrapSDKContext(ctx), key, nil, req.Namespace, func(tick uint64, batch []byte) bool {
		// we keep the check here so that if we hit the limit,
		// we return the NEXT key in the iteration, not the one before it.
		if count == limit {
			res.Page.Key = k.bytesForUint(tick)
			return false
		}
		res.Batches = append(res.Batches, &types.TransactionBatch{
			Namespace: req.Namespace,
			Tick:      tick,
			Batch:     batch,
		})
		count++
		return true
	})
	return &res, nil
}
