package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &MsgClaimQuestReward{}
)

func (msg MsgClaimQuestReward) ValidateBasic() error {
	if msg.UserId == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("user_id cannot be empty")
	}
	if msg.QuestId == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("quest_id cannot be empty")
	}
	return nil
}

// GetSigners implements sdk.Msg
func (msg MsgClaimQuestReward) GetSigners() []sdk.AccAddress {
	accAddr, err := sdk.AccAddressFromBech32(msg.UserId)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{accAddr}
}

func NewMsgClaimQuestReward(userId, questId string) MsgClaimQuestReward {
	return MsgClaimQuestReward{
		UserId:  userId,
		QuestId: questId,
	}
}
