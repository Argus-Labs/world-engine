package types

import (
	"net"
	"regexp"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg              = &UpdateNamespaceRequest{}
	_ sdk.HasValidateBasic = &UpdateNamespaceRequest{}
)

var alphanumeric = regexp.MustCompile("^[a-zA-Z0-9_]*$")

func isAlphaNumeric(s string) bool {
	return alphanumeric.MatchString(s)
}

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
	if !isAlphaNumeric(ns.ShardName) {
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
