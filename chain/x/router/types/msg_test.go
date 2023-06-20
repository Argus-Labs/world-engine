package types

import (
	"net"
	"testing"

	"github.com/btcsuite/btcutil/bech32"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"gotest.tools/v3/assert"
)

func TestUpdateNamespaceRequest(t *testing.T) {
	validAddr := "cosmos1luyncewxk4lm24k6gqy8y5dxkj0klr4tu0lmnj"
	testCases := []struct {
		name   string
		msg    UpdateNamespaceRequest
		expErr error
	}{
		{
			name: "valid",
			msg: UpdateNamespaceRequest{
				Authority:    validAddr,
				ShardName:    "foo",
				ShardAddress: "127.0.0.0:3000",
			},
		},
		{
			name: "valid TLD",
			msg: UpdateNamespaceRequest{
				Authority:    validAddr,
				ShardName:    "foo",
				ShardAddress: "cosmos.sdk.io:3000",
			},
		},
		{
			name: "invalid addr",
			msg: UpdateNamespaceRequest{
				Authority: "blah",
			},
			expErr: bech32.ErrInvalidLength(4),
		},
		{
			name: "empty shard name",
			msg: UpdateNamespaceRequest{
				Authority: validAddr,
			},
			expErr: sdkerrors.ErrInvalidRequest,
		},
		{
			name: "cant use non-alphanumeric in shardnames",
			msg: UpdateNamespaceRequest{
				Authority: validAddr,
				ShardName: "foo.bar-4",
			},
			expErr: sdkerrors.ErrInvalidRequest,
		},
		{
			name: "empty shard address",
			msg: UpdateNamespaceRequest{
				Authority: validAddr,
				ShardName: "foo",
			},
			expErr: sdkerrors.ErrInvalidRequest,
		},
		{
			name: "invalid shard addr",
			msg: UpdateNamespaceRequest{
				Authority:    validAddr,
				ShardName:    "foo",
				ShardAddress: "blah",
			},
			expErr: &net.AddrError{Addr: "blah", Err: "missing port in address"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.msg.ValidateBasic()
			if tc.expErr != nil {
				assert.ErrorContains(t, err, tc.expErr.Error())
			} else {
				assert.NilError(t, err)
			}
		})
	}
}
