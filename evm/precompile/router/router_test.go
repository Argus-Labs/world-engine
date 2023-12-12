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

package router

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	"testing"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"pkg.berachain.dev/polaris/eth/accounts/abi"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/lib/utils"
	generated "pkg.world.dev/world-engine/evm/precompile/contracts/bindings/cosmos/precompile/router"
)

type RouterTestSuite struct {
	suite.Suite
	sf       *ethprecompile.StatefulFactory
	contract *Contract
}

func TestRouter(t *testing.T) {
	suite.Run(t, &RouterTestSuite{})
}

func (r *RouterTestSuite) SetupTest() {
	r.contract = utils.MustGetAs[*Contract](
		NewPrecompileContract(
			nil,
		),
	)
	r.sf = ethprecompile.NewStatefulFactory()
}

func (r *RouterTestSuite) TestStaticRegistryKey() {
	r.Require().Equal(r.contract.RegistryKey(), common.BytesToAddress(authtypes.NewModuleAddress(name)))
}

func (r *RouterTestSuite) TestABIMethods() {
	var contractABI abi.ABI
	err := contractABI.UnmarshalJSON([]byte(generated.RouterMetaData.ABI))
	r.Require().NoError(err)
	r.Require().Equal(r.contract.ABIMethods(), contractABI.Methods)
}

func (r *RouterTestSuite) TestMatchPrecompileMethods() {
	_, err := r.sf.Build(r.contract, nil)
	r.Require().NoError(err)
}

func (r *RouterTestSuite) TestCustomValueDecoderIsNoop() {
	r.Require().Nil(r.contract.CustomValueDecoders())
}
