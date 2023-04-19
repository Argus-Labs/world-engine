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

package governance

import (
	"context"
	"math/big"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	v1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile"
	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	"pkg.berachain.dev/polaris/cosmos/precompile"
	"pkg.berachain.dev/polaris/eth/common"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/lib/utils"
)

// Contract is the precompile contract for the governance module.
type Contract struct {
	precompile.BaseContract

	msgServer v1.MsgServer
	querier   v1.QueryServer
}

// NewPrecompileContract creates a new precompile contract for the governance module.
func NewPrecompileContract(gk *govkeeper.Keeper) ethprecompile.StatefulImpl {
	return &Contract{
		BaseContract: precompile.NewBaseContract(
			generated.GovernanceModuleMetaData.ABI,
			cosmlib.AccAddressToEthAddress(authtypes.NewModuleAddress(govtypes.ModuleName)),
		),
		msgServer: govkeeper.NewMsgServerImpl(gk),
		querier:   gk,
	}
}

// PrecompileMethods implements the `ethprecompile.StatefulImpl` interface.
func (c *Contract) PrecompileMethods() ethprecompile.Methods {
	return ethprecompile.Methods{
		{
			AbiSig:  "submitProposal(bytes,(uint64,string)[],string,string,string,bool)",
			Execute: c.SubmitProposal,
		},
		{
			AbiSig:  "cancelProposal(uint64)",
			Execute: c.CancelProposal,
		},
		{
			AbiSig:  "vote(uint64,int32,string)",
			Execute: c.Vote,
		},
		{
			AbiSig:  "voteWeighted(uint64,(int32,string)[],string)",
			Execute: c.VoteWeighted,
		},
		{
			AbiSig:  "getProposal(uint64)",
			Execute: c.GetProposal,
		},
		{
			AbiSig:  "getProposals(int32)",
			Execute: c.GetProposals,
		},
	}
}

// SubmitProposal is the method for the `submitProposal` method of the governance precompile contract.
func (c *Contract) SubmitProposal(
	ctx context.Context,
	_ ethprecompile.EVM,
	caller common.Address,
	value *big.Int,
	readonly bool,
	args ...any,
) ([]any, error) {
	message, ok := utils.GetAs[[]*codectypes.Any](args[0]) // TODO: check if this is the correct type.
	if !ok {
		return nil, precompile.ErrInvalidAny
	}
	initialDeposit, ok := utils.GetAs[[]generated.IGovernanceModuleCoin](args[1])
	if !ok {
		return nil, precompile.ErrInvalidCoin
	}
	metadata, ok := utils.GetAs[string](args[2])
	if !ok {
		return nil, precompile.ErrInvalidString
	}
	title, ok := utils.GetAs[string](args[3])
	if !ok {
		return nil, precompile.ErrInvalidString
	}
	summary, ok := utils.GetAs[string](args[4])
	if !ok {
		return nil, precompile.ErrInvalidString
	}
	expedited, ok := utils.GetAs[bool](args[5])
	if !ok {
		return nil, precompile.ErrInvalidBool
	}

	// Caller is the proposer (msg.sender).
	proposer := sdk.AccAddress(caller.Bytes())

	return c.submitProposalHelper(ctx, message, initialDeposit, proposer, metadata, title, summary, expedited)
}

// CancelProposal is the method for the `cancelProposal` method of the governance precompile contract.
func (c *Contract) CancelProposal(
	ctx context.Context,
	_ ethprecompile.EVM,
	caller common.Address,
	value *big.Int,
	readonly bool,
	args ...any,
) ([]any, error) {
	id, ok := utils.GetAs[uint64](args[0])
	if !ok {
		return nil, precompile.ErrInvalidUint64
	}
	proposer := sdk.AccAddress(caller.Bytes())

	return c.cancelProposalHelper(ctx, proposer, id)
}

// Vote is the method for the `vote` method of the governance precompile contract.
func (c *Contract) Vote(
	ctx context.Context,
	_ ethprecompile.EVM,
	caller common.Address,
	value *big.Int,
	readonly bool,
	args ...any,
) ([]any, error) {
	proposalID, ok := utils.GetAs[uint64](args[0])
	if !ok {
		return nil, precompile.ErrInvalidUint64
	}
	options, ok := utils.GetAs[int32](args[1])
	if !ok {
		return nil, precompile.ErrInvalidInt32
	}
	metadata, ok := utils.GetAs[string](args[2])
	if !ok {
		return nil, precompile.ErrInvalidString
	}
	voter := sdk.AccAddress(caller.Bytes())

	return c.voteHelper(ctx, voter, proposalID, options, metadata)
}

// VoteWeighted is the method for the `voteWeighted` method of the governance precompile contract.
func (c *Contract) VoteWeighted(
	ctx context.Context,
	_ ethprecompile.EVM,
	caller common.Address,
	value *big.Int,
	readonly bool,
	args ...any,
) ([]any, error) {
	proposalID, ok := utils.GetAs[uint64](args[0])
	if !ok {
		return nil, precompile.ErrInvalidBigInt
	}
	options, ok := utils.GetAs[[]generated.IGovernanceModuleWeightedVoteOption](args[1])
	if !ok {
		return nil, precompile.ErrInvalidOptions
	}
	metadata, ok := utils.GetAs[string](args[2])
	if !ok {
		return nil, precompile.ErrInvalidString
	}
	voter := sdk.AccAddress(caller.Bytes())
	return c.voteWeightedHelper(ctx, voter, proposalID, options, metadata)
}

// GetProposal is the method for the `getProposal` method of the governance precompile contract.
func (c *Contract) GetProposal(
	ctx context.Context,
	_ ethprecompile.EVM,
	caller common.Address,
	value *big.Int,
	readonly bool,
	args ...any,
) ([]any, error) {
	proposalID, ok := utils.GetAs[uint64](args[0])
	if !ok {
		return nil, precompile.ErrInvalidUint64
	}

	return c.getProposalHelper(ctx, proposalID)
}

// GetProposals is the method for the `getProposal` method of the governance precompile contract.
func (c *Contract) GetProposals(
	ctx context.Context,
	_ ethprecompile.EVM,
	caller common.Address,
	value *big.Int,
	readonly bool,
	args ...any,
) ([]any, error) {
	proposalStatus, ok := utils.GetAs[int32](args[0])
	if !ok {
		return nil, precompile.ErrInvalidInt32
	}

	return c.getProposalsHelper(ctx, proposalStatus)
}
