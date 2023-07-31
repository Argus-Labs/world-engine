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
	"context"
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile/staking"
	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	testutil "pkg.berachain.dev/polaris/cosmos/testing/utils"
	"pkg.berachain.dev/polaris/eth/accounts/abi"
	"pkg.berachain.dev/polaris/eth/common"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/eth/core/vm"
	"pkg.berachain.dev/polaris/eth/core/vm/mock"
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

		sdkCtx sdk.Context

		contract *Contract

		sf *ethprecompile.StatefulFactory
	)

	BeforeEach(func() {
		sdkCtx, _, bk, sk = testutil.SetupMinimalKeepers()
		skPtr := &sk
		contract = utils.MustGetAs[*Contract](NewPrecompileContract(skPtr))
		sf = ethprecompile.NewStatefulFactory()
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
			_, err := sf.Build(contract, nil)
			Expect(err).ToNot(HaveOccurred())
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
			del            sdk.AccAddress
			val            sdk.ValAddress
			validator      stakingtypes.Validator
			otherValidator stakingtypes.Validator
			otherVal       sdk.ValAddress
			caller         common.Address
			mockEVM        *mock.PrecompileEVMMock
			ctx            context.Context
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

			otherValidator, err = NewValidator(otherVal, PKs[1])
			Expect(err).ToNot(HaveOccurred())

			validator, _ = validator.AddTokensFromDel(sdkmath.NewIntFromBigInt(amount))
			otherValidator, _ = otherValidator.AddTokensFromDel(sdkmath.NewIntFromBigInt(amount))

			mockEVM = mock.NewEVM()
			ctx = vm.NewPolarContext(sdkCtx, mockEVM, caller, big.NewInt(0))

			validator = stakingkeeper.TestingUpdateValidator(
				&sk,
				sdk.UnwrapSDKContext(vm.UnwrapPolarContext(ctx).Context()),
				validator,
				true,
			)
			otherValidator = stakingkeeper.TestingUpdateValidator(
				&sk,
				sdk.UnwrapSDKContext(vm.UnwrapPolarContext(ctx).Context()),
				otherValidator,
				true,
			)

			delegation := stakingtypes.NewDelegation(del.String(), val.String(), sdkmath.LegacyNewDec(9))
			Expect(sk.SetDelegation(ctx, delegation)).To(Succeed())

			// Check that the delegation was created.
			res, err := sk.GetDelegation(ctx, del, val)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(delegation))

			// Set the denom.
			defaultParams := stakingtypes.DefaultParams()
			defaultParams.BondDenom = "stake"
			err = sk.SetParams(ctx, defaultParams)
			Expect(err).ToNot(HaveOccurred())

		})

		When("Delegate", func() {

			It("should succeed", func() {
				amountToDelegate, ok := new(big.Int).SetString("22000000000000000000", 10)
				Expect(ok).To(BeTrue())
				err := FundAccount(
					sdk.UnwrapSDKContext(vm.UnwrapPolarContext(ctx).Context()),
					bk,
					del,
					sdk.NewCoins(
						sdk.NewCoin(
							"stake",
							sdkmath.NewIntFromBigInt(amountToDelegate),
						),
					),
				)
				Expect(err).ToNot(HaveOccurred())

				_, err = contract.Delegate(
					ctx,
					cosmlib.ValAddressToEthAddress(val),
					amountToDelegate,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("GetDelegation", func() {

			It("should return the correct delegation", func() {
				res, err := contract.GetDelegation(
					ctx,
					cosmlib.AccAddressToEthAddress(del), cosmlib.ValAddressToEthAddress(val),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(big.NewInt(9))) // should have correct shares
			})
		})

		When("Undelegate", func() {

			It("should succeed", func() {
				_, err := contract.Undelegate(
					ctx,
					cosmlib.ValAddressToEthAddress(val),
					big.NewInt(1),
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("BeginRedelegations", func() {

			It("should succeed", func() {
				_, err := contract.BeginRedelegate(
					ctx,
					cosmlib.ValAddressToEthAddress(val),
					cosmlib.ValAddressToEthAddress(otherVal),
					big.NewInt(1),
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("CancelUnbondingDelegation", func() {
			It("should succeed", func() {
				creationHeight := sdk.UnwrapSDKContext(vm.UnwrapPolarContext(ctx).Context()).BlockHeight()
				amount, ok := new(big.Int).SetString("1", 10)
				Expect(ok).To(BeTrue())

				// Undelegate.
				_, err := contract.Undelegate(
					ctx,
					cosmlib.ValAddressToEthAddress(val),
					amount,
				)
				Expect(err).ToNot(HaveOccurred())

				_, err = contract.CancelUnbondingDelegation(
					ctx,
					cosmlib.ValAddressToEthAddress(val),
					amount,
					creationHeight,
				)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("GetUnbondingDelegation", func() {

			It("should succeed", func() {
				// Undelegate.
				amount, ok := new(big.Int).SetString("1", 10)
				Expect(ok).To(BeTrue())
				_, err := contract.Undelegate(
					ctx,
					cosmlib.ValAddressToEthAddress(val),
					amount,
				)
				Expect(err).ToNot(HaveOccurred())

				res, err := contract.GetUnbondingDelegation(
					ctx,
					caller,
					cosmlib.ValAddressToEthAddress(val),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})
		})

		When("GetRedelegations", func() {
			It("should succeed", func() {

				amount, ok := new(big.Int).SetString("220000000000000000000", 10)
				Expect(ok).To(BeTrue())

				err := FundAccount(
					sdk.UnwrapSDKContext(vm.UnwrapPolarContext(ctx).Context()),
					bk,
					cosmlib.AddressToAccAddress(caller),
					sdk.NewCoins(
						sdk.NewCoin(
							"stake",
							sdkmath.NewIntFromBigInt(amount),
						),
					),
				)
				Expect(err).ToNot(HaveOccurred())

				validator.Status = stakingtypes.Bonded
				Expect(sk.SetValidator(ctx, validator)).To(Succeed())

				ret, err := contract.Delegate(
					ctx,
					cosmlib.ValAddressToEthAddress(val),
					amount,
				)
				Expect(ret).To(BeTrue())
				Expect(err).ToNot(HaveOccurred())

				del, err := contract.GetDelegation(ctx,
					caller,
					cosmlib.ValAddressToEthAddress(val),
				)
				Expect(err).ToNot(HaveOccurred())

				Expect(del.Cmp(new(big.Int).Add(amount, big.NewInt(9)))).To(Equal(0))

				otherValidator.Status = stakingtypes.Bonded

				Expect(sk.SetValidator(ctx, otherValidator)).To(Succeed())

				ret, err = contract.BeginRedelegate(
					ctx,
					cosmlib.ValAddressToEthAddress(val),
					cosmlib.ValAddressToEthAddress(otherVal),
					amount,
				)
				Expect(ret).To(BeTrue())
				Expect(err).ToNot(HaveOccurred())

				del, err = contract.GetDelegation(ctx,
					caller,
					cosmlib.ValAddressToEthAddress(val),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect((del).Cmp(big.NewInt(9))).To(Equal(0))

				del, err = contract.GetDelegation(ctx,
					caller,
					cosmlib.ValAddressToEthAddress(otherVal),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect((del).Cmp(amount)).To(Equal(0))

				redels, err := contract.GetRedelegations(
					ctx,
					caller,
					cosmlib.ValAddressToEthAddress(val),
					cosmlib.ValAddressToEthAddress(otherVal),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(redels).ToNot(BeNil())
			})
		})

		When("GetRedelegations0", func() {
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
						Expect(vals.Cmp(big.NewInt(0))).To(Equal(0))
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
						Expect(vals).To(BeEmpty())
					})

					It("should succeed", func() {
						// Undelegate.
						amount, ok := new(big.Int).SetString("1", 10)
						Expect(ok).To(BeTrue())
						_, err := contract.Undelegate(
							ctx,
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

						_, err := contract.BeginRedelegate(
							ctx,
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
				Expect(sk.SetValidator(ctx, validator)).To(Succeed())

				// Get the active validators.
				res, err := contract.GetActiveValidators(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(HaveLen(1))
				Expect(res[0]).To(Equal(cosmlib.ValAddressToEthAddress(val)))
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
