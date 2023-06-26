package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg              = &SubmitBatchRequest{}
	_ sdk.HasValidateBasic = &SubmitBatchRequest{}
)

func (m *SubmitBatchRequest) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Sender); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrap(err.Error())
	}
	if m.Batch == nil {
		return sdkerrors.ErrInvalidRequest.Wrap("batch cannot be empty")
	}
	return nil
}
