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
	"testing"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile/router"
	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	"pkg.berachain.dev/polaris/eth/accounts/abi"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/lib/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAddressPrecompile(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cosmos/precompile/router")
}

var _ = Describe("Address Precompile", func() {
	var contract *Contract
	var sf *ethprecompile.StatefulFactory
	BeforeEach(func() {
		contract = utils.MustGetAs[*Contract](
			NewPrecompileContract(
				nil,
			),
		)
		sf = ethprecompile.NewStatefulFactory()
	})

	It("should have static registry key", func() {
		Expect(contract.RegistryKey()).To(Equal(
			cosmlib.AccAddressToEthAddress(authtypes.NewModuleAddress(name))),
		)
	})

	It("should have correct ABI methods", func() {
		var cAbi abi.ABI
		err := cAbi.UnmarshalJSON([]byte(generated.RouterMetaData.ABI))
		Expect(err).ToNot(HaveOccurred())
		Expect(contract.ABIMethods()).To(Equal(cAbi.Methods))
	})

	It("should match the precompile methods", func() {
		_, err := sf.Build(contract, nil)
		Expect(err).ToNot(HaveOccurred())
	})

	It("custom value decoder should be no-op", func() {
		Expect(contract.CustomValueDecoders()).To(BeNil())
	})

})
