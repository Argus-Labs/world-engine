package vm

import sdk "github.com/cosmos/cosmos-sdk/types"

type AllowlistCheck func(ctx sdk.Context, addr string) bool

type ContractAllowlistOption struct {
	check AllowlistCheck
}

func NewContractAllowlistOption(check AllowlistCheck) ContractAllowlistOption {
	return ContractAllowlistOption{check: check}
}

func (c *ContractAllowlistOption) CanCreate(ctx sdk.Context, addr string) bool {
	return c.check(ctx, addr)
}
