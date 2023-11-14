package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg              = &SubmitShardTxRequest{}
	_ sdk.HasValidateBasic = &SubmitShardTxRequest{}
)

func (m *SubmitShardTxRequest) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Sender); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrap(err.Error())
	}
	return nil
}
