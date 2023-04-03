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

package configuration

import (
	"context"

	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"pkg.berachain.dev/polaris/cosmos/x/evm/plugins"
	"pkg.berachain.dev/polaris/cosmos/x/evm/types"
	"pkg.berachain.dev/polaris/eth/common"
	"pkg.berachain.dev/polaris/eth/core"
	"pkg.berachain.dev/polaris/eth/params"
)

// Plugin is the interface that must be implemented by the plugin.
type Plugin interface {
	plugins.BaseCosmosPolaris
	core.ConfigurationPlugin
	SetParams(params *types.Params)
	GetParams() *types.Params
	GetEvmDenom() string
}

// plugin implements the core.ConfigurationPlugin interface.
type plugin struct {
	storeKey    storetypes.StoreKey
	paramsStore storetypes.KVStore
	evmDenom    string
}

// NewPlugin returns a new plugin instance.
func NewPlugin(storeKey storetypes.StoreKey) Plugin {
	return &plugin{
		storeKey: storeKey,
	}
}

// Prepare implements the core.ConfigurationPlugin interface.
func (p *plugin) Prepare(ctx context.Context) {
	sCtx := sdk.UnwrapSDKContext(ctx)
	p.paramsStore = sCtx.KVStore(p.storeKey)
}

func (p *plugin) GetEvmDenom() string {
	if p.evmDenom == "" {
		p.evmDenom = p.GetParams().EvmDenom
	}
	return p.evmDenom
}

// ChainConfig implements the core.ConfigurationPlugin interface.
func (p *plugin) ChainConfig() *params.ChainConfig {
	return p.GetParams().EthereumChainConfig()
}

// ExtraEips implements the core.ConfigurationPlugin interface.
func (p *plugin) ExtraEips() []int {
	eips := make([]int, 0)
	for _, e := range p.GetParams().ExtraEIPs {
		eips = append(eips, int(e))
	}
	return eips
}

// FeeCollector implements the core.ConfigurationPlugin interface.
func (p *plugin) FeeCollector() *common.Address {
	// TODO: parameterize fee collector name.
	addr := common.BytesToAddress([]byte(authtypes.FeeCollectorName))
	return &addr
}
