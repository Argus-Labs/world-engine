package keeper

import (
	"testing"

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

	res, err := suite.k.Namespaces(suite.ctx, &types.NamespacesRequest{})
	assert.NilError(t, err)
	assert.Equal(t, len(res.Namespaces), 1)
	assert.Equal(t, res.Namespaces[0].ShardName, msg.ShardName)
	assert.Equal(t, res.Namespaces[0].ShardAddress, msg.ShardAddress)

	// bad authority shouldn't pass
	msg.Authority = "oops"
	_, err = suite.k.UpdateNamespace(suite.ctx, msg)
	assert.ErrorIs(t, err, sdkerrors.ErrUnauthorized)
}
