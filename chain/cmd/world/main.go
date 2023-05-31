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

	"github.com/cosmos/cosmos-sdk/server"
	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"
	"github.com/spf13/viper"

	"github.com/argus-labs/world-engine/chain/cmd/world/cmd"
	"github.com/argus-labs/world-engine/chain/config"
	simapp "github.com/argus-labs/world-engine/chain/runtime"
	runtimeconfig "github.com/argus-labs/world-engine/chain/runtime/config"
)

func main() {
	worldEngineCfg, err := getWorldEngineConfig()
	if err != nil {
		panic(err)
	}
	runtimeconfig.SetupCosmosConfig(worldEngineCfg)

	rootCmd := cmd.NewRootCmd()
	if err := svrcmd.Execute(rootCmd, "", simapp.DefaultNodeHome); err != nil {
		//nolint: errorlint // uhh fix?
		switch e := err.(type) {
		case server.ErrorCode:
			os.Exit(e.Code)

		default:
			os.Exit(1)
		}
	}
}

// getWorldEngineConfig loads the world engine configuration. It requires that a path and filename be in
// the environment variables, so that viper can target the file and load it.
func getWorldEngineConfig() (config.WorldEngineConfig, error) {
	path := os.Getenv("WORLD_ENGINE_CONFIG_PATH")
	name := os.Getenv("WORLD_ENGINE_CONFIG_NAME")
	viper.AddConfigPath(path)
	viper.SetConfigName(name)
	err := viper.ReadInConfig()
	if err != nil {
		return config.WorldEngineConfig{}, err
	}
	worldEngineCfg := config.WorldEngineConfig{}
	err = viper.Unmarshal(&worldEngineCfg)
	if err != nil {
		return config.WorldEngineConfig{}, err
	}
	return worldEngineCfg, nil
}
