package keeper

import (
	"testing"

	"cosmossdk.io/orm/types/ormerrors"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/chain/x/router/types"
)

func TestNamespaces(t *testing.T) {
	auth := "0xFooBar"
	suite := setupBase(t, auth)

	msg := &types.UpdateNamespaceRequest{
		Authority:    auth,
		ShardName:    "foo",
		ShardAddress: "127.0.0.0:1532",
	}
	_, err := suite.k.UpdateNamespace(suite.ctx, msg)
	assert.NilError(t, err)

	msg2 := &types.UpdateNamespaceRequest{
		Authority:    auth,
		ShardName:    "bar",
		ShardAddress: "localhost:5321",
	}
	_, err = suite.k.UpdateNamespace(suite.ctx, msg2)
	assert.NilError(t, err)

	res, err := suite.k.Namespaces(suite.ctx, &types.NamespacesRequest{})
	assert.NilError(t, err)
	assert.Equal(t, len(res.Namespaces), 2)
	assert.Equal(t, res.Namespaces[1].ShardName, msg.ShardName)
	assert.Equal(t, res.Namespaces[1].ShardAddress, msg.ShardAddress)
	assert.Equal(t, res.Namespaces[0].ShardName, msg2.ShardName)
	assert.Equal(t, res.Namespaces[0].ShardAddress, msg2.ShardAddress)

	// bad authority shouldn't pass
	msg.Authority = "oops"
	_, err = suite.k.UpdateNamespace(suite.ctx, msg)
	assert.ErrorIs(t, err, sdkerrors.ErrUnauthorized)
}

func TestAddress(t *testing.T) {
	auth := "0xFooBar"
	suite := setupBase(t, auth)

	msg := &types.UpdateNamespaceRequest{
		Authority:    auth,
		ShardName:    "foo",
		ShardAddress: "127.0.0.0:1532",
	}
	_, err := suite.k.UpdateNamespace(suite.ctx, msg)
	assert.NilError(t, err)

	res, err := suite.k.Address(suite.ctx, &types.AddressRequest{Namespace: msg.ShardName})
	assert.NilError(t, err)
	assert.Equal(t, res.Address, msg.ShardAddress)

	_, err = suite.k.Address(suite.ctx, &types.AddressRequest{Namespace: "meow"})
	assert.ErrorIs(t, err, ormerrors.NotFound)
}
