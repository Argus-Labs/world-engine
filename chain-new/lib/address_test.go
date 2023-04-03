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

package lib_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	"pkg.berachain.dev/polaris/eth/common"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Address", func() {
	It("should return the correct address", func() {
		addr := common.HexToAddress("0xCd8c4Cb0C7f93a2B74B3e522a1C7BE35bE1Fbc73")
		bech32 := "cosmos1ekxyevx8lyazka9nu532r3a7xklpl0rnjrc2a9"
		acc, err := sdk.AccAddressFromBech32(bech32)
		Expect(err).NotTo(HaveOccurred())
		addr2 := cosmlib.AccAddressToEthAddress(acc)
		Expect(addr.String()).To(Equal(addr2.String()))
		valAddr1 := cosmlib.ValAddressToEthAddress(sdk.ValAddress(acc))
		Expect(addr.String()).To(Equal(valAddr1.String()))

		ethAddr := cosmlib.AddressToAccAddress(addr)
		bech322 := sdk.MustBech32ifyAddressBytes(sdk.GetConfig().GetBech32AccountAddrPrefix(), ethAddr.Bytes())
		Expect(bech322).To(Equal(bech32))

		ethAddr2 := cosmlib.AddressToValAddress(addr)
		bech3222 := sdk.MustBech32ifyAddressBytes(sdk.GetConfig().GetBech32ValidatorAddrPrefix(), ethAddr2.Bytes())
		Expect(bech3222).To(Equal("cosmosvaloper1ekxyevx8lyazka9nu532r3a7xklpl0rnhhvl3k"))

	})
})
