// SPDX-License-Identifier: BUSL-1.1
//
// Copyright (C) 2023, Berachain Foundation. All rights reserved.
// Use of this software is govered by the Business Source License included
// in the LICENSE file of this repository and at www.mariadb.com/bsl11.
//
// ANY USE OF THE LICENSED WORK IN VIOLATION OF THIS LICENSE WILL AUTOMATICALLY
// TERMINATE YOUR RIGHTS UNDER THIS LICENSE FOR THE CURRENT AND ALL OTHER
// VERSIONS OF THE LICENSED WORK.
//
// THIS LICENSE DOES NOT GRANT YOU ANY RIGHT IN ANY TRADEMARK OR LOGO OF
// LICENSOR OR ITS AFFILIATES (PROVIDED THAT YOU MAY USE A TRADEMARK OR LOGO OF
// LICENSOR AS EXPRESSLY REQUIRED BY THIS LICENSE).
//
// TO THE EXTENT PERMITTED BY APPLICABLE LAW, THE LICENSED WORK IS PROVIDED ON
// AN “AS IS” BASIS. LICENSOR HEREBY DISCLAIMS ALL WARRANTIES AND CONDITIONS,
// EXPRESS OR IMPLIED, INCLUDING (WITHOUT LIMITATION) WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, NON-INFRINGEMENT, AND
// TITLE.

package distribution

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"

	"pkg.berachain.dev/polaris/contracts/bindings/cosmos/lib"
	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile/distribution"
	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	"pkg.berachain.dev/polaris/cosmos/precompile/staking"
	"pkg.berachain.dev/polaris/eth/common"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/eth/core/vm"
)

// Contract is the precompile contract for the distribution module.
type Contract struct {
	ethprecompile.BaseContract

	vs        staking.ValidatorStore
	msgServer distributiontypes.MsgServer
	querier   distributiontypes.QueryServer
}

// NewPrecompileContract returns a new instance of the distribution module precompile contract.
func NewPrecompileContract(
	vs staking.ValidatorStore, m distributiontypes.MsgServer, q distributiontypes.QueryServer,
) *Contract {
	return &Contract{
		BaseContract: ethprecompile.NewBaseContract(
			generated.DistributionModuleMetaData.ABI,
			common.BytesToAddress([]byte{0x69}),
		),
		vs:        vs,
		msgServer: m,
		querier:   q,
	}
}

func (c *Contract) CustomValueDecoders() ethprecompile.ValueDecoders {
	return ethprecompile.ValueDecoders{
		distributiontypes.AttributeKeyValidator: c.ConvertValAddressFromBech32,
	}
}

// SetWithdrawAddress is the precompile contract method for the `setWithdrawAddress(address)` method.
func (c *Contract) SetWithdrawAddress(
	ctx context.Context,
	withdrawAddress common.Address,
) (bool, error) {
	return c.setWithdrawAddressHelper(
		ctx,
		sdk.AccAddress(vm.UnwrapPolarContext(ctx).MsgSender().Bytes()),
		sdk.AccAddress(withdrawAddress.Bytes()),
	)
}

// GetWithdrawEnabled is the precompile contract method for the `getWithdrawEnabled()` method.
func (c *Contract) GetWithdrawEnabled(
	ctx context.Context,
) (bool, error) {
	return c.getWithdrawAddrEnabled(ctx)
}

// WithdrawDelegatorReward is the precompile contract method for the
// `withdrawDelegatorReward(address,address)` method.
func (c *Contract) WithdrawDelegatorReward(
	ctx context.Context,
	delegator common.Address,
	validator common.Address,
) ([]lib.CosmosCoin, error) {
	return c.withdrawDelegatorRewardsHelper(
		ctx,
		sdk.AccAddress(delegator.Bytes()),
		sdk.ValAddress(validator.Bytes()),
	)
}

// ConvertValAddressFromBech32 converts a bech32 string representing a validator address to a
// common.Address.
func (c *Contract) ConvertValAddressFromBech32(attributeValue string) (any, error) {
	// extract the sdk.ValAddress from string value
	return cosmlib.EthAddressFromString(c.vs.ValidatorAddressCodec(), attributeValue)
}
