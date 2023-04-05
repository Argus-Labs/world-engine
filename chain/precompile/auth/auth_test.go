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

package auth_test

import (
	"context"
	"math/big"
	"testing"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile"
	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	"pkg.berachain.dev/polaris/cosmos/precompile"
	"pkg.berachain.dev/polaris/cosmos/precompile/auth"
	"pkg.berachain.dev/polaris/eth/accounts/abi"
	"pkg.berachain.dev/polaris/eth/common"
	"pkg.berachain.dev/polaris/lib/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAddressPrecompile(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cosmos/precompile/auth")
}

var _ = Describe("Address Precompile", func() {
	var contract *auth.Contract

	BeforeEach(func() {
		contract = utils.MustGetAs[*auth.Contract](auth.NewPrecompileContract())
	})

	It("should have static registry key", func() {
		Expect(contract.RegistryKey()).To(Equal(
			cosmlib.AccAddressToEthAddress(authtypes.NewModuleAddress(authtypes.ModuleName))),
		)
	})

	It("should have correct ABI methods", func() {
		var cAbi abi.ABI
		err := cAbi.UnmarshalJSON([]byte(generated.AuthModuleMetaData.ABI))
		Expect(err).ToNot(HaveOccurred())
		Expect(contract.ABIMethods()).To(Equal(cAbi.Methods))
	})

	It("should match the precompile methods", func() {
		Expect(contract.PrecompileMethods()).To(HaveLen(len(contract.ABIMethods())))
	})

	It("custom value decoder should be no-op", func() {
		Expect(contract.CustomValueDecoders()).To(BeNil())
	})

	When("When Calling ConvertHexToBech32", func() {
		It("should fail on invalid inputs", func() {
			res, err := contract.ConvertHexToBech32(
				context.Background(),
				nil,
				common.Address{},
				big.NewInt(0),
				false,
				"invalid",
			)
			Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
			Expect(res).To(BeNil())
		})

		It("should not convert from invalid hex to bech32", func() {
			res, err := contract.ConvertHexToBech32(
				context.Background(),
				nil,
				common.Address{},
				big.NewInt(0),
				false,
				common.BytesToAddress([]byte("test")),
			)
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})
	})
	When("Calling ConvertBech32ToHexAddress", func() {
		It("should error if invalid type", func() {
			res, err := contract.ConvertBech32ToHexAddress(
				context.Background(),
				nil,
				common.Address{},
				big.NewInt(0),
				false,
				common.BytesToAddress([]byte("invalid")),
			)
			Expect(err).To(MatchError(precompile.ErrInvalidString))
			Expect(res).To(BeNil())
		})

		It("should error if invalid bech32 address", func() {
			res, err := contract.ConvertBech32ToHexAddress(
				context.Background(),
				nil,
				common.Address{},
				big.NewInt(0),
				false,
				"0xxxxx",
			)
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())
		})

		It("should convert from bech32 to hex", func() {
			res, err := contract.ConvertBech32ToHexAddress(
				context.Background(),
				nil,
				common.Address{},
				big.NewInt(0),
				false,
				cosmlib.AddressToAccAddress(common.BytesToAddress([]byte("test"))).String(),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(res[0]).To(Equal(common.BytesToAddress([]byte("test"))))
		})
	})
})
