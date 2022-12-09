package v1

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &MsgClaimQuestReward{}
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

func NewMsgClaimQuestReward(userId, questId string) MsgClaimQuestReward {
	return MsgClaimQuestReward{
		User_ID:  userId,
		Quest_ID: questId,
	}
}
