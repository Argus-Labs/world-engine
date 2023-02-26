package rollup

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	serverConfig "github.com/cosmos/cosmos-sdk/server/config"

	rollconf "github.com/rollkit/rollkit/config"

	argus "github.com/argus-labs/argus/app"
)

type Config struct {
	appCfg  argus.AppConfig
	sCfg    serverConfig.Config
	sCtx    server.Context
	cCtx    client.Context
	rollCfg rollconf.NodeConfig
}

func DefaultConfig() Config {
	/*
		the values that worked...
			serverCtx := server.NewDefaultContext()
			serverCtx.Config.Genesis = "someGenesis.json" // TODO(technicallyty): this should come after config refactor WORLD-75
			clientCtx := client.Context{}
			serverCfg := serverconfig.DefaultConfig()
			serverCfg.MinGasPrices = "100stake" // TODO(technicallyty): this should come after config refactor WORLD-75
			rollConfig := rollconf.NodeConfig{}
	*/
	sCtx := server.NewDefaultContext()
	cCtx := client.Context{}
	sCfg := serverConfig.DefaultConfig()
	rCfg := rollconf.NodeConfig{}
	cfg := Config{
		appCfg:  argus.AppConfig{},
		sCfg:    *sCfg,
		sCtx:    *sCtx,
		cCtx:    cCtx,
		rollCfg: rCfg,
	}
	return cfg
}
