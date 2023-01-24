package v1

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/migrations/legacytx"
)

var (
	_ sdk.Msg            = &MsgClaimQuestReward{}
	_ legacytx.LegacyMsg = &MsgClaimQuestReward{}

	_ sdk.Msg = &MsgAllowContractCreation{}
)

func (m MsgClaimQuestReward) ValidateBasic() error {
	if m.User_ID == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("user_id cannot be empty")
	}
	if m.Quest_ID == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("quest_id cannot be empty")
	}
	return nil
}

// GetSigners implements sdk.Msg
func (m MsgClaimQuestReward) GetSigners() []sdk.AccAddress {
	accAddr, err := sdk.AccAddressFromBech32(m.User_ID)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{accAddr}
}

func (m MsgClaimQuestReward) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&m))
}

func (m MsgClaimQuestReward) Route() string {
	return sdk.MsgTypeURL(&m)
}

func (m MsgClaimQuestReward) Type() string {
	return sdk.MsgTypeURL(&m)
}

func NewMsgClaimQuestReward(userID, questID string) MsgClaimQuestReward {
	return MsgClaimQuestReward{
		User_ID:  userID,
		Quest_ID: questID,
	}
}

func (m *MsgAllowContractCreation) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Addr); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrap("sender")
	}
	return nil
}

func (m *MsgAllowContractCreation) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Sender)
	return []sdk.AccAddress{addr}
}
