package types

import (
	"net"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/argus-labs/world-engine/chain/utils"
)

var (
	_ sdk.Msg              = &UpdateNamespaceRequest{}
	_ sdk.HasValidateBasic = &UpdateNamespaceRequest{}
)

func (m *UpdateNamespaceRequest) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return err
	}
	if m.Namespace == nil {
		return sdkerrors.ErrInvalidRequest.Wrap("namespace cannot be empty")
	}
	return m.Namespace.Validate()
}

func (ns *Namespace) Validate() error {
	if ns.ShardName == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("shard name cannot be empty")
	}
	if !utils.IsAlphaNumeric(ns.ShardName) {
		return sdkerrors.ErrInvalidRequest.Wrap("shard name must only contain alphanumeric characters")
	}
	if ns.ShardAddress == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("shard address cannot be empty")
	}
	host, port, err := net.SplitHostPort(ns.ShardAddress)
	if err != nil {
		return err
	}
	if host == "" || port == "" {
		return sdkerrors.ErrInvalidRequest.Wrapf("%s is not a valid address", ns.ShardAddress)
	}
	return nil
}
