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

package staking

import (
	"math/big"
	"testing"

	"cosmossdk.io/math"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile"
	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	"pkg.berachain.dev/polaris/cosmos/precompile"
	testutil "pkg.berachain.dev/polaris/cosmos/testing/utils"
	"pkg.berachain.dev/polaris/eth/accounts/abi"
	"pkg.berachain.dev/polaris/eth/common"
	"pkg.berachain.dev/polaris/lib/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStakingPrecompile(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cosmos/precompile/staking")
}

func createValAddrs(count int) ([]sdk.AccAddress, []sdk.ValAddress) {
	addrs := simtestutil.CreateIncrementalAccounts(count)
	valAddrs := simtestutil.ConvertAddrsToValAddrs(addrs)

	return addrs, valAddrs
}

func NewValidator(operator sdk.ValAddress, pubKey cryptotypes.PubKey) (stakingtypes.Validator, error) {
	return stakingtypes.NewValidator(operator, pubKey, stakingtypes.Description{})
}

var (
	PKs = simtestutil.CreateTestPubKeys(500)
)

var _ = Describe("Staking", func() {
	var (
		sk stakingkeeper.Keeper
		bk bankkeeper.BaseKeeper

		ctx sdk.Context

		contract *Contract
	)

	BeforeEach(func() {
		ctx, _, bk, sk = testutil.SetupMinimalKeepers()
		skPtr := &sk
		contract = utils.MustGetAs[*Contract](NewPrecompileContract(skPtr))
	})

	When("AbiMethods", func() {
		It("returns the correct methods", func() {
			var cAbi abi.ABI
			err := cAbi.UnmarshalJSON([]byte(generated.StakingModuleMetaData.ABI))
			Expect(err).ToNot(HaveOccurred())
			methods := contract.ABIMethods()
			Expect(methods).To(HaveLen(len(cAbi.Methods)))
		})
	})

	When("PrecompileMethods", func() {
		It("should return the correct methods", func() {
			Expect(contract.PrecompileMethods()).To(HaveLen(len(contract.ABIMethods())))
		})
	})

	When("ABIEvents", func() {
		It("should return the correct events", func() {
			var cAbi abi.ABI
			err := cAbi.UnmarshalJSON([]byte(generated.StakingModuleMetaData.ABI))
			Expect(err).ToNot(HaveOccurred())
			events := contract.ABIEvents()
			Expect(events).To(HaveLen(len(cAbi.Events)))
		})
	})

	When("CustomValueDecoders", func() {
		It("should be a no-op", func() {
			Expect(contract.CustomValueDecoders()).To(BeNil())
		})
	})

	When("Calling Precompile Methods", func() {
		var (
			del       sdk.AccAddress
			val       sdk.ValAddress
			validator stakingtypes.Validator
			otherVal  sdk.ValAddress
			caller    common.Address
		)

		BeforeEach(func() {
			delegates, validators := createValAddrs(2)
			del, val, otherVal = delegates[0], validators[0], validators[1]
			caller = cosmlib.AccAddressToEthAddress(del)

			amount, ok := new(big.Int).SetString("22000000000000000000", 10) // 22 tokens.
			Expect(ok).To(BeTrue())
			var err error
			validator, err = NewValidator(val, PKs[0])
			Expect(err).ToNot(HaveOccurred())
			otherValidator, err := NewValidator(otherVal, PKs[1])
			Expect(err).ToNot(HaveOccurred())
			validator, _ = validator.AddTokensFromDel(sdk.NewIntFromBigInt(amount))
			otherValidator, _ = otherValidator.AddTokensFromDel(sdk.NewIntFromBigInt(amount))
			validator = stakingkeeper.TestingUpdateValidator(&sk, ctx, validator, true)
			stakingkeeper.TestingUpdateValidator(&sk, ctx, otherValidator, true)

			delegation := stakingtypes.NewDelegation(del, val, math.LegacyNewDec(9))
			sk.SetDelegation(ctx, delegation)

			// Check that the delegation was created.
			res, found := sk.GetDelegation(ctx, del, val)
			Expect(found).To(BeTrue())
			Expect(res).To(Equal(delegation))

			// Set the denom.
			defaultParams := stakingtypes.DefaultParams()
			defaultParams.BondDenom = "stake"
			err = sk.SetParams(ctx, defaultParams)
			Expect(err).ToNot(HaveOccurred())

		})

		When("DelegateAddrInput", func() {
			It("should fail if input is not a common.Address", func() {
				res, err := contract.DelegateAddrInput(
					ctx,
					nil,
					caller,
					big.NewInt(0),
					true,
					"0x",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})

			It("should fail if the amount is not a *big.Int", func() {
				res, err := contract.DelegateAddrInput(
					ctx,
					nil,
					caller,
					big.NewInt(0),
					true,
					cosmlib.ValAddressToEthAddress(val),
					"amount",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidBigInt))
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				amountToDelegate, ok := new(big.Int).SetString("22000000000000000000", 10)
				Expect(ok).To(BeTrue())
				err := FundAccount(
					ctx,
					bk,
					del,
					sdk.NewCoins(
						sdk.NewCoin(
							"stake",
							sdk.NewIntFromBigInt(amountToDelegate),
						),
					),
				)
				Expect(err).ToNot(HaveOccurred())

				_, err = contract.DelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					cosmlib.ValAddressToEthAddress(val),
					amountToDelegate,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("DelegateStringInput", func() {
			It("should fail if input is not a string", func() {
				res, err := contract.DelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					90909,
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should fail if the amount is not a *big.Int", func() {
				res, err := contract.DelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					val.String(),
					"amount",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidBigInt))
				Expect(res).To(BeNil())
			})

			It("should fail if the string is not a valid address", func() {
				res, err := contract.DelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					"invalid",
					big.NewInt(0),
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				amountToDelegate, ok := new(big.Int).SetString("22000000000000000000", 10)
				Expect(ok).To(BeTrue())
				err := FundAccount(
					ctx,
					bk,
					del,
					sdk.NewCoins(
						sdk.NewCoin(
							"stake",
							sdk.NewIntFromBigInt(amountToDelegate),
						),
					),
				)
				Expect(err).ToNot(HaveOccurred())

				_, err = contract.DelegateStringInput(
					ctx,
					nil,
					caller,
					big.NewInt(0),
					false,
					val.String(),
					amountToDelegate,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("GetDelegationAddrInput", func() {
			It("should return an error if the input del is not a common.Address", func() {
				res, err := contract.GetDelegationAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					"0x", cosmlib.ValAddressToEthAddress(val),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})

			It("should return an error if the val address is not common.address", func() {
				res, err := contract.GetDelegationAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					cosmlib.AccAddressToEthAddress(del), "0x",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})

			It("should return the correct delegation", func() {
				res, err := contract.GetDelegationAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					cosmlib.AccAddressToEthAddress(del), cosmlib.ValAddressToEthAddress(val),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(res[0]).To(Equal(big.NewInt(9))) // should have correct shares
			})
		})

		When("GetDelegationStringInput", func() {
			It("should error if not string", func() {
				res, err := contract.GetDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					0, val.String(),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should error if the val address is not a string", func() {
				res, err := contract.GetDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					del.String(), 0,
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should error if del not bech32", func() {
				res, err := contract.GetDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					"0x", val.String(),
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			It("should return an error if the val is not bech32", func() {
				res, err := contract.GetDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					del.String(), "0x",
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			It("should return the correct delegation", func() {
				res, err := contract.GetDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					del.String(), val.String(),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(res[0]).To(Equal(big.NewInt(9))) // should have correct shares
			})
		})

		When("UndelegateAddrInput", func() {
			It("should fail if the input is not a common.Address", func() {
				res, err := contract.UndelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					"0x",
					big.NewInt(0),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})

			It("should fail if the amount is not a *big.Int", func() {
				res, err := contract.UndelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					cosmlib.ValAddressToEthAddress(val),
					"amount",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidBigInt))
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				_, err := contract.UndelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					cosmlib.ValAddressToEthAddress(val),
					big.NewInt(1),
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("UndelegateStringInput", func() {
			It("should fail if the input is not a string", func() {
				res, err := contract.UndelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					90909,
					big.NewInt(0),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should fail if the amount is not a *big.Int", func() {
				res, err := contract.UndelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					val.String(),
					"amount",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidBigInt))
				Expect(res).To(BeNil())
			})

			It("should fail if the address is not of type bech32", func() {
				res, err := contract.UndelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					"0x",
					big.NewInt(0),
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				_, err := contract.UndelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					val.String(),
					big.NewInt(1),
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("BeginRedelegationsAddrInput", func() {
			It("should fail if the srcValue is not a common.Address", func() {
				res, err := contract.BeginRedelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					10,
					cosmlib.ValAddressToEthAddress(val),
					big.NewInt(1),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})

			It("should fail if the dstValue is not a common.Address", func() {
				res, err := contract.BeginRedelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					10,
					big.NewInt(1),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})

			It("should fail if the amount is not a *big.Int", func() {
				res, err := contract.BeginRedelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					cosmlib.ValAddressToEthAddress(val),
					"amount",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidBigInt))
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				_, err := contract.BeginRedelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					cosmlib.ValAddressToEthAddress(otherVal),
					big.NewInt(1),
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("BeginRedelegationsStringInput", func() {
			It("should fail if the srcValue is not a string", func() {
				res, err := contract.BeginRedelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					10,
					val.String(),
					big.NewInt(1),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should fail if the dstValue is not a string", func() {
				res, err := contract.BeginRedelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					val.String(),
					10,
					big.NewInt(1),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should fail if the amount is not a *big.Int", func() {
				res, err := contract.BeginRedelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					val.String(),
					otherVal.String(),
					"amount",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidBigInt))
				Expect(res).To(BeNil())
			})

			It("should fail if the srcValue is not of type bech32", func() {
				res, err := contract.BeginRedelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					"0x",
					val.String(),
					big.NewInt(1),
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			It("should fail if the dstValue is not of type bech32", func() {
				res, err := contract.BeginRedelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					val.String(),
					"0x",
					big.NewInt(1),
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				_, err := contract.BeginRedelegateStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					val.String(),
					otherVal.String(),
					big.NewInt(1),
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("CancelUnbondingDelegationAddrInput", func() {
			It("should fail if the address is not a common.Address", func() {
				res, err := contract.CancelUnbondingDelegationAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					"cosmlib.ValAddressToEthAddress(val)",
					big.NewInt(1),
					int64(1),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})

			It("should fail if the amount is not a *big.Int", func() {
				res, err := contract.CancelUnbondingDelegationAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					"amount",
					int64(1),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidBigInt))
				Expect(res).To(BeNil())
			})

			It("should fail if creation height is not an int64", func() {
				res, err := contract.CancelUnbondingDelegationAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					big.NewInt(1),
					"height",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidInt64))
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				creationHeight := ctx.BlockHeight()
				amount, ok := new(big.Int).SetString("1", 10)
				Expect(ok).To(BeTrue())

				// Undelegate.
				_, err := contract.UndelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					amount,
				)
				Expect(err).ToNot(HaveOccurred())

				_, err = contract.CancelUnbondingDelegationAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					amount,
					creationHeight,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("CancelUnbondingDelegationStringInput", func() {
			It("should fail if the address is not a string", func() {
				res, err := contract.CancelUnbondingDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					10,
					big.NewInt(1),
					int64(1),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should fail if the amount is not a *big.Int", func() {
				res, err := contract.CancelUnbondingDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					val.String(),
					"amount",
					int64(1),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidBigInt))
				Expect(res).To(BeNil())
			})

			It("should fail if creation height is not an int64", func() {
				res, err := contract.CancelUnbondingDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					val.String(),
					big.NewInt(1),
					"height",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidInt64))
				Expect(res).To(BeNil())
			})

			It("should fail if the address is not a bech32 address", func() {
				res, err := contract.CancelUnbondingDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					"0x",
					big.NewInt(1),
					int64(1),
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				creationHeight := ctx.BlockHeight()
				amount, ok := new(big.Int).SetString("1", 10)
				Expect(ok).To(BeTrue())

				// Undelegate.
				_, err := contract.UndelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					amount,
				)
				Expect(err).ToNot(HaveOccurred())

				_, err = contract.CancelUnbondingDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					val.String(),
					amount,
					creationHeight,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("GetUnbondingDelegationAddrInput", func() {
			It("should fail if address is not a common.Address", func() {
				res, err := contract.GetUnbondingDelegationAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					caller,
					"cosmlib.ValAddressToEthAddress(val)",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				// Undelegate.
				amount, ok := new(big.Int).SetString("1", 10)
				Expect(ok).To(BeTrue())
				_, err := contract.UndelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					amount,
				)
				Expect(err).ToNot(HaveOccurred())

				res, err := contract.GetUnbondingDelegationAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					caller,
					cosmlib.ValAddressToEthAddress(val),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})
		})

		When("GetUnbondingDelegationStringInput", func() {
			It("should fail if address is not a string", func() {
				res, err := contract.GetUnbondingDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					10,
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should fail if address is not a bech32 address", func() {
				res, err := contract.GetUnbondingDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					"0x",
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			It("should succeed", func() {
				// Undelegate.
				amount, ok := new(big.Int).SetString("1", 10)
				Expect(ok).To(BeTrue())
				_, err := contract.UndelegateAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					false,
					cosmlib.ValAddressToEthAddress(val),
					amount,
				)
				Expect(err).ToNot(HaveOccurred())

				res, err := contract.GetUnbondingDelegationStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					cosmlib.AddressToAccAddress(caller).String(),
					val.String(),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})
		})

		When("GetRedelegationsAddrInput", func() {
			It("should fail if address is not a common.Address", func() {
				res, err := contract.GetRedelegationsAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					caller,
					"cosmlib.ValAddressToEthAddress(val)",
					cosmlib.ValAddressToEthAddress(val),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})

			It("should fail if dst address is not a common.Address", func() {
				res, err := contract.GetRedelegationsAddrInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					cosmlib.ValAddressToEthAddress(val),
					"cosmlib.ValAddressToEthAddress(val)",
				)
				Expect(err).To(MatchError(precompile.ErrInvalidHexAddress))
				Expect(res).To(BeNil())
			})
		})

		When("GetRedelegationsStringInput", func() {
			It("should fail if src address is not a string", func() {
				res, err := contract.GetRedelegationsStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					10,
					otherVal.String(),
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should fail if dst address is not a string", func() {
				res, err := contract.GetRedelegationsStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					val.String(),
					10,
				)
				Expect(err).To(MatchError(precompile.ErrInvalidString))
				Expect(res).To(BeNil())
			})

			It("should fail if src address is not a bech32 address", func() {
				res, err := contract.GetRedelegationsStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					cosmlib.AddressToAccAddress(caller).String(),
					"0x",
					otherVal.String(),
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			It("should fail if dst address is not a bech32 address", func() {
				res, err := contract.GetRedelegationsStringInput(
					ctx, nil, caller,
					big.NewInt(0),
					true,
					cosmlib.AddressToAccAddress(caller).String(),
					val.String(),
					"0x",
				)
				Expect(err).To(HaveOccurred())
				Expect(res).To(BeNil())
			})

			When("Calling Helper Methods", func() {
				When("delegationHelper", func() {
					It("should fail if the del address is not valid", func() {
						_, err := contract.getDelegationHelper(
							ctx,
							sdk.AccAddress(""),
							val,
						)
						Expect(err).To(HaveOccurred())
					})
					It("should fail if the val address is not valid", func() {
						_, err := contract.getDelegationHelper(
							ctx,
							del,
							sdk.ValAddress(""),
						)
						Expect(err).To(HaveOccurred())
					})
					It("should not error if there is no delegation", func() {
						vals, err := contract.getDelegationHelper(
							ctx,
							del,
							otherVal,
						)
						Expect(err).ToNot(HaveOccurred())
						del := utils.MustGetAs[*big.Int](vals[0])
						Expect(del.Cmp(big.NewInt(0))).To(Equal(0))
					})
					It("should succeed", func() {
						_, err := contract.getDelegationHelper(
							ctx,
							del,
							val,
						)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("getUnbondingDelegationHelper", func() {
					It("should fail if caller address is wrong", func() {
						_, err := contract.getUnbondingDelegationHelper(
							ctx,
							sdk.AccAddress([]byte("")),
							val,
						)
						Expect(err).To(HaveOccurred())
					})

					It("should fail if there is no unbonding delegation", func() {
						vals, err := contract.getUnbondingDelegationHelper(
							ctx,
							cosmlib.AddressToAccAddress(caller),
							otherVal,
						)
						Expect(err).ToNot(HaveOccurred())
						_, ok := utils.GetAs[[]stakingtypes.UnbondingDelegationEntry](vals[0])
						Expect(ok).To(BeTrue())
					})

					It("should succeed", func() {
						// Undelegate.
						amount, ok := new(big.Int).SetString("1", 10)
						Expect(ok).To(BeTrue())
						_, err := contract.UndelegateAddrInput(
							ctx,
							nil,
							caller,
							big.NewInt(0),
							false,
							cosmlib.ValAddressToEthAddress(val),
							amount,
						)
						Expect(err).ToNot(HaveOccurred())

						_, err = contract.getUnbondingDelegationHelper(
							ctx,
							cosmlib.AddressToAccAddress(caller),
							val,
						)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("getRedelegationHelper", func() {
					It("should fail if caller address is wrong", func() {
						_, err := contract.getRedelegationsHelper(
							ctx,
							sdk.AccAddress([]byte("")),
							val,
							otherVal,
						)
						Expect(err).To(HaveOccurred())
					})

					It("should fail if there is no redelegation", func() {
						_, err := contract.getRedelegationsHelper(
							ctx,
							cosmlib.AddressToAccAddress(caller),
							val,
							otherVal,
						)
						Expect(err).To(HaveOccurred())
					})

					It("should succeed", func() {
						// Redelegate.
						amount, ok := new(big.Int).SetString("1", 10)
						Expect(ok).To(BeTrue())

						_, err := contract.BeginRedelegateAddrInput(
							ctx,
							nil,
							caller,
							big.NewInt(0),
							false,
							cosmlib.ValAddressToEthAddress(val),
							cosmlib.ValAddressToEthAddress(otherVal),
							amount,
						)
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})
		})

		When("GetActiveValidators", func() {
			It("gets active validators", func() {
				// Set the validator to be bonded.
				validator.Status = stakingtypes.Bonded
				sk.SetValidator(ctx, validator)

				// Get the active validators.
				res, err := contract.GetActiveValidators(ctx, nil, caller, big.NewInt(0), true)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(HaveLen(1))
				addrs := utils.MustGetAs[[]common.Address](res[0])
				Expect(addrs[0]).To(Equal(cosmlib.ValAddressToEthAddress(val)))
			})
		})
	})
})

func FundAccount(ctx sdk.Context, bk bankkeeper.BaseKeeper, account sdk.AccAddress, coins sdk.Coins) error {
	if err := bk.MintCoins(ctx, stakingtypes.ModuleName, coins); err != nil {
		return err
	}
	return bk.SendCoinsFromModuleToAccount(ctx, stakingtypes.ModuleName, account, coins)
}
