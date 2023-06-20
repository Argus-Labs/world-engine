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

package mock

import (
	"math/big"

	"github.com/holiman/uint256"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"

	gethvm "github.com/ethereum/go-ethereum/core/vm"

	"pkg.berachain.dev/polaris/eth/common"
	"pkg.berachain.dev/polaris/eth/core/vm"
)

type PrecompileEVM interface {
	GetStateDB() gethvm.StateDB

	Call(
		caller vm.ContractRef,
		addr common.Address,
		input []byte,
		gas uint64,
		value *big.Int,
	) (ret []byte, leftOverGas uint64, err error)
	StaticCall(
		caller vm.ContractRef,
		addr common.Address,
		input []byte,
		gas uint64,
	) (ret []byte, leftOverGas uint64, err error)
	Create(
		caller vm.ContractRef,
		code []byte,
		gas uint64,
		value *big.Int,
	) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error)
	Create2(
		caller vm.ContractRef,
		code []byte,
		gas uint64,
		endowment *big.Int,
		salt *uint256.Int,
	) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error)
	GetContext() *vm.BlockContext
}

type MessageRouter interface {
	Handler(msg sdk.Msg) baseapp.MsgServiceHandler
	HandlerByTypeURL(typeURL string) baseapp.MsgServiceHandler
}
