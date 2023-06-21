package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/argus-labs/world-engine/chain/x/router/types"
)

var _ types.QueryServiceServer = &Keeper{}

func (k *Keeper) Namespaces(ctx context.Context, _ *types.NamespacesRequest) (*types.NamespacesResponse, error) {
	namespaces := k.getAllNamespaces(sdk.UnwrapSDKContext(ctx))

	return &types.NamespacesResponse{Namespaces: namespaces}, nil
}

func (k *Keeper) Address(ctx context.Context, request *types.AddressRequest) (*types.AddressResponse, error) {
	addr, err := k.getAddressForNamespace(sdk.UnwrapSDKContext(ctx), request.Namespace)
	if err != nil {
		return nil, err
	}
	return &types.AddressResponse{Address: addr}, nil
}
