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

package jsonrpc

import (
	"context"
	"math/big"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	gethrpc "github.com/ethereum/go-ethereum/rpc"

	"pkg.berachain.dev/polaris/cosmos/testing/network"
	"pkg.berachain.dev/polaris/eth/common"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "pkg.berachain.dev/polaris/cosmos/testing/integration/utils"
)

var _ = Describe("Network", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should connect -- multiple clients", func() {
		// Dial an Ethereum RPC Endpoint
		rpcClient, err := gethrpc.DialContext(ctx, tf.Network.Validators[0].APIAddress+"/eth/rpc")
		Expect(err).ToNot(HaveOccurred())
		c := ethclient.NewClient(rpcClient)
		Expect(err).ToNot(HaveOccurred())
		Expect(c).ToNot(BeNil())
	})

	It("should support eth_chainId", func() {
		chainID, err := client.ChainID(ctx)
		Expect(chainID.String()).To(Equal("69420"))
		Expect(err).ToNot(HaveOccurred())
	})

	It("should support eth_gasPrice", func() {

		gasPrice, err := tf.EthClient.SuggestGasPrice(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(gasPrice).ToNot(BeNil())
	})

	It("should support eth_blockNumber", func() {
		// Get the latest block
		blockNumber, err := client.BlockNumber(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(blockNumber).To(BeNumerically(">", 0))
	})

	It("should support eth_getBalance", func() {
		// Get the balance of an account
		balance, err := client.BalanceAt(ctx, network.TestAddress, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(balance.Uint64()).To(BeNumerically(">", 0))
	})

	It("should support eth_estimateGas", func() {
		// Estimate the gas required for a transaction
		from := network.TestAddress
		to := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
		value := big.NewInt(1000000000000)

		msg := geth.CallMsg{
			From:  from,
			To:    &to,
			Value: value,
		}

		gas, err := client.EstimateGas(ctx, msg)
		Expect(err).ToNot(HaveOccurred())
		Expect(gas).To(BeNumerically(">", 0))
	})

	It("should deploy, mint tokens and check balance, eth_getTransactionByHash", func() {
		// Deploy the contract
		erc20Contract := DeployERC20(tf.GenerateTransactOpts(""), client)

		// Mint tokens
		tx, err := erc20Contract.Mint(tf.GenerateTransactOpts(""),
			network.TestAddress, big.NewInt(100000000))
		Expect(err).ToNot(HaveOccurred())

		// Get the transaction by its hash, it should be pending here.
		txHash := tx.Hash()

		// Wait for it to be mined.
		ExpectMined(client, tx)
		ExpectSuccessReceipt(client, tx)

		// Get the transaction by its hash, it should be mined here.
		fetchedTx, isPending, err := client.TransactionByHash(ctx, txHash)
		Expect(err).ToNot(HaveOccurred())
		Expect(isPending).To(BeFalse())
		Expect(fetchedTx.Hash()).To(Equal(txHash))

		// Check the erc20 balance
		erc20Balance, err := erc20Contract.BalanceOf(&bind.CallOpts{}, network.TestAddress)
		Expect(err).ToNot(HaveOccurred())
		Expect(erc20Balance).To(Equal(big.NewInt(100000000)))
	})

})
