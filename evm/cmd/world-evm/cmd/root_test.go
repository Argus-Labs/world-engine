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

package cmd_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/cosmos/cosmos-sdk/client/flags"
	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	simapp "pkg.world.dev/world-engine/evm/app"
	"pkg.world.dev/world-engine/evm/cmd/world-evm/cmd"
)

func TestCmd(t *testing.T) {
	sdk.GetConfig().SetBech32PrefixForAccount("world", "world")
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "chain/cmd/world/cmd:integration")
}

var _ = ginkgo.Describe("Init command", func() {
	ginkgo.It("should initialize the app with given options", func() {
		stdout := os.Stdout
		defer func() { os.Stdout = stdout }()
		os.Stdout = os.NewFile(0, os.DevNull)
		rootCmd := cmd.NewRootCmd()
		rootCmd.SetArgs([]string{
			"init",        // Test the init cmd
			"simapp-test", // Moniker
			fmt.Sprintf("--%s=%s", cli.FlagOverwrite, "true"), // Overwrite genesis.json, in case it already exists
		})

		err := svrcmd.Execute(rootCmd, "", simapp.DefaultNodeHome)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})
})

var _ = ginkgo.Describe("Home flag registration", func() {
	ginkgo.It("should set home directory correctly", func() {
		// Redirect standard out to null
		stdout := os.Stdout
		defer func() { os.Stdout = stdout }()
		os.Stdout = os.NewFile(0, os.DevNull)
		homeDir := os.TempDir()

		rootCmd := cmd.NewRootCmd()
		rootCmd.SetArgs([]string{
			"query",
			fmt.Sprintf("--%s", flags.FlagHome),
			homeDir,
		})

		err := svrcmd.Execute(rootCmd, "", simapp.DefaultNodeHome)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		result, err := rootCmd.Flags().GetString(flags.FlagHome)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(result).To(gomega.Equal(homeDir))
	})
})
