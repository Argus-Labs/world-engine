package types

import (
	"net"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &UpdateNamespaceRequest{}
)

func (m *UpdateNamespaceRequest) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return err
	}
	if m.ShardName == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("shard name cannot be empty")
	}
	if m.ShardAddress == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("shard address cannot be empty")
	}
	host, port, err := net.SplitHostPort(m.ShardAddress)
	if err != nil {
		return err
	}
	if host == "" || port == "" {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s is not a valid address", m.ShardAddress)
	}
	return nil
}

func (m *UpdateNamespaceRequest) GetSigners() []sdk.AccAddress {
	addr := sdk.MustAccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}
