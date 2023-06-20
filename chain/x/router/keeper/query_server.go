package keeper

import (
	"context"

	routerv1 "github.com/argus-labs/world-engine/chain/api/router/v1"
	"github.com/argus-labs/world-engine/chain/x/router/types"
)

func (k *Keeper) Namespaces(ctx context.Context, request *types.NamespacesRequest) (*types.NamespacesResponse, error) {
	nameSpaces := make([]*types.Namespace, 0, 5)
	it, err := k.store.NamespaceTable().List(ctx, routerv1.NamespaceShardNameIndexKey{})
	if err != nil {
		return nil, err
	}
	for it.Next() {
		ns, err := it.Value()
		if err != nil {
			return nil, err
		}
		nameSpaces = append(nameSpaces, &types.Namespace{
			ShardName:    ns.ShardName,
			ShardAddress: ns.ShardAddress,
		})
	}

	return &types.NamespacesResponse{Namespaces: nameSpaces}, nil
}
