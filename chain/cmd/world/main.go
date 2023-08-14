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

package main

import (
	"os"

	"cosmossdk.io/log"
	"github.com/spf13/viper"

	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"

	simapp "github.com/argus-labs/world-engine/chain/app"
	"github.com/argus-labs/world-engine/chain/cmd/world/cmd"
	"github.com/argus-labs/world-engine/chain/config"
	"github.com/argus-labs/world-engine/chain/types"
)

func main() {
	var err error
	var cfg config.WorldEngineConfig
	cfg, err = getWorldEngineConfig()
	if err != nil {
		cfg = getDefaultConfig()
	}
	types.SetupCosmosConfig(cfg)
	rootCmd := cmd.NewRootCmd()
	if err = svrcmd.Execute(rootCmd, "", simapp.DefaultNodeHome); err != nil {
		log.NewLogger(rootCmd.OutOrStderr()).Error("failure when running app", "err", err)
		os.Exit(1)
	}
}

func getDefaultConfig() config.WorldEngineConfig {
	return config.WorldEngineConfig{
		DisplayDenom:    "stake",
		BaseDenom:       "ustake",
		Bech32Prefix:    "polar",
		RouterAuthority: "",
	}
}

// getWorldEngineConfig loads the world engine configuration. It requires that a path and filename be in
// the environment variables, so that viper can target the file and load it.
func getWorldEngineConfig() (config.WorldEngineConfig, error) {
	v := viper.New()
	path := os.Getenv("WORLD_ENGINE_CONFIG_PATH")
	name := os.Getenv("WORLD_ENGINE_CONFIG_NAME")
	v.AddConfigPath(path)
	v.SetConfigName(name)
	err := v.ReadInConfig()
	if err != nil {
		return config.WorldEngineConfig{}, err
	}
	worldEngineCfg := config.WorldEngineConfig{}
	err = v.Unmarshal(&worldEngineCfg)
	if err != nil {
		return config.WorldEngineConfig{}, err
	}
	return worldEngineCfg, nil
}
