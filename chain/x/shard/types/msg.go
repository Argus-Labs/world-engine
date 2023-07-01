package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/argus-labs/world-engine/chain/utils"
)

var (
	_ sdk.Msg              = &SubmitBatchRequest{}
	_ sdk.HasValidateBasic = &SubmitBatchRequest{}
)

func (m *SubmitBatchRequest) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Sender); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrap(err.Error())
	}
	return m.TransactionBatch.Validate()
}

func (tb *TransactionBatch) Validate() error {
	if len(tb.Namespace) == 0 || !utils.IsAlphaNumeric(tb.Namespace) {
		return sdkerrors.ErrInvalidRequest.Wrap("invalid namespace. must be a non-empty alphanumeric string")
	}
	if tb.Batch == nil || len(tb.Batch) == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("batch cannot be empty")
	}
	return nil
}
