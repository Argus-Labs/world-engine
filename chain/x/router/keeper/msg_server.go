package keeper

import (
	"context"

	routerv1 "github.com/argus-labs/world-engine/chain/api/router/v1"
	"github.com/argus-labs/world-engine/chain/x/router/types"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ types.MsgServiceServer = &Keeper{}

func (k *Keeper) UpdateNamespace(ctx context.Context, request *types.UpdateNamespaceRequest) (*types.UpdateNamespaceResponse, error) {
	if k.authority != request.Authority {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("%s is not allowed to update namespaces, expected %s", request.Authority, k.authority)
	}

	err := k.store.NamespaceTable().Save(ctx, &routerv1.Namespace{
		ShardName:    request.ShardName,
		ShardAddress: request.ShardAddress,
	})

	return &types.UpdateNamespaceResponse{}, err
}
