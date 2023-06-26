package types

import (
	"testing"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"gotest.tools/v3/assert"
)

func TestSubmitBatchMsg(t *testing.T) {
	validAddr := "cosmos1luyncewxk4lm24k6gqy8y5dxkj0klr4tu0lmnj"
	testCases := []struct {
		name   string
		msg    SubmitBatchRequest
		expErr error
	}{
		{
			name: "valid",
			msg: SubmitBatchRequest{
				Sender: validAddr,
				Batch:  []byte("batch"),
			},
		},
		{
			name: "invalid signer",
			msg: SubmitBatchRequest{
				Sender: "foo",
			},
			expErr: sdkerrors.ErrInvalidAddress,
		},
		{
			name: "nil batch",
			msg: SubmitBatchRequest{
				Sender: validAddr,
				Batch:  nil,
			},
			expErr: sdkerrors.ErrInvalidRequest,
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
