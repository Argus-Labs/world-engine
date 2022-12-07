package v1

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &MsgClaimQuestReward{}
)

func (m MsgClaimQuestReward) ValidateBasic() error {
	if m.UserId == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("user_id cannot be empty")
	}
	if m.QuestId == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("quest_id cannot be empty")
	}
	return nil
}

// GetSigners implements sdk.Msg
func (m MsgClaimQuestReward) GetSigners() []sdk.AccAddress {
	accAddr, err := sdk.AccAddressFromBech32(m.UserId)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{accAddr}
}

func NewMsgClaimQuestReward(userID, questID string) MsgClaimQuestReward {
	return MsgClaimQuestReward{
		UserId:  userID,
		QuestId: questID,
	}
}
