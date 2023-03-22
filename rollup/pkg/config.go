package pkg

import (
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	serverConfig "github.com/cosmos/cosmos-sdk/server/config"
	"github.com/spf13/viper"

	rollconf "github.com/rollkit/rollkit/config"

	argus "github.com/argus-labs/argus/app"
)

type Config struct {
	AppCfg    argus.AppConfig     `mapstructure:"app_cfg"`
	ServerCfg serverConfig.Config `mapstructure:"server_cfg"`
	ServerCtx server.Context      `mapstructure:"server_ctx"`
	ClientCtx client.Context      `mapstructure:"client_ctx"`
	Rollup    rollconf.NodeConfig `mapstructure:"rollup_cfg"`
}

func LoadConfig(configName string) (*Config, error) {
	v := viper.New()
	v.SetConfigName(configName)
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("couldn't load config: %s", err)
	}
	c := DefaultConfig()
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("couldn't read config: %s", err)
	}
	daCfg := os.Getenv("da_config")
	if daCfg != "" {
		fmt.Println("setting da config")
		c.Rollup.DAConfig = daCfg
	} else {
		fmt.Println("da config did not update")
	}
	return &c, nil
}

func DefaultConfig() Config {
	/*
		the values that worked...
			serverCtx := server.NewDefaultContext()
			serverCtx.Config.Genesis = "example_genesis.json" // TODO(technicallyty): this should come after config refactor WORLD-75
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
		AppCfg:    argus.AppConfig{},
		ServerCfg: *sCfg,
		ServerCtx: *sCtx,
		ClientCtx: cCtx,
		Rollup:    rCfg,
	}
	return cfg
}
