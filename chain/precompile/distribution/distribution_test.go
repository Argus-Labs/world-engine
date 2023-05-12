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

package distribution_test

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"gotest.tools/v3/assert"

	"pkg.berachain.dev/polaris/cosmos/precompile/distribution"
	"pkg.berachain.dev/polaris/cosmos/x/evm/plugins/precompile/log"
	"pkg.berachain.dev/polaris/eth/core/vm"
	"pkg.berachain.dev/polaris/lib/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

/*
solidity struct:
   struct MsgSendEnergy {
       uint to;
       uint from;
       uint amount;
   }
*/

type MsgSendEnergy struct {
	To     *big.Int `json:"to"`
	From   *big.Int `json:"from"`
	Amount *big.Int `json:"amount"`
}

func TestDecoding(t *testing.T) {

	// bytes from abi.encoding the following solidity struct:
	//    struct MsgSendEnergy {
	//        uint to;
	//        uint from;
	//        uint amount;
	//    }
	bzStr := "0x0000000000000000000000000000000000000000000000000000000000000cdd00000000000000000000000000000000000000000000000000000000000009510000000000000000000000000000000000000000000000000000000000005bd5"
	bz, err := hexutil.Decode(bzStr)
	assert.NilError(t, err)

	// make MsgSendEnergy abi type.
	msgSendEnergyType, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "to", Type: "uint256"},
		{Name: "from", Type: "uint256"},
		{Name: "amount", Type: "uint256"},
	})
	assert.NilError(t, err)
	msgSendEnergyType.TupleType = reflect.TypeOf(MsgSendEnergy{})

	args := abi.Arguments{{Type: msgSendEnergyType}}
	unpacked, err := args.Unpack(bz)
	assert.NilError(t, err)

	goodMsg, ok := unpacked[0].(MsgSendEnergy)
	assert.Check(t, ok == true)
	assert.Check(t, goodMsg.From.Int64() > 0 && goodMsg.To.Int64() > 0 && goodMsg.Amount.Int64() > 0)

	bz, err = args.Pack(goodMsg)
	assert.NilError(t, err)

	unpacked, err = args.Unpack(bz)
	assert.NilError(t, err)
	goodMsg, ok = unpacked[0].(MsgSendEnergy)
	assert.Check(t, ok == true)
	fmt.Println(goodMsg)
}

func TestDistributionPrecompile(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cosmos/precompile/distribution")
}

var _ = Describe("Distribution Precompile Test", func() {
	var contract *distribution.Contract
	var valAddr sdk.ValAddress
	var f *log.Factory
	var amt sdk.Coin

	BeforeEach(func() {
		valAddr = sdk.ValAddress([]byte("val"))
		amt = sdk.NewCoin("denom", sdk.NewInt(100))

		contract = utils.MustGetAs[*distribution.Contract](distribution.NewPrecompileContract())

		// Register the events.
		f = log.NewFactory([]vm.RegistrablePrecompile{contract})
	})

	It("should register the withdraw event", func() {
		event := sdk.NewEvent(
			distributiontypes.EventTypeWithdrawRewards,
			sdk.NewAttribute(distributiontypes.AttributeKeyValidator, valAddr.String()),
			sdk.NewAttribute(sdk.AttributeKeyAmount, amt.String()),
		)

		log, err := f.Build(&event)
		Expect(err).ToNot(HaveOccurred())
		Expect(log.Address).To(Equal(contract.RegistryKey()))
	})
})
