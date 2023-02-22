package rollup

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"

	argus "github.com/argus-labs/argus/app"
	"github.com/argus-labs/argus/cmd/argusd/cmd"
)

var _ Application = app{}

func NewApplication() Application {
	return app{}
}

type app struct {
	evmHooks []EVMNakamaHook
	chain    argus.ArgusApp
}

// Start does starting things
//
// TODO(technicallyty): implement
func (a app) Start() error {
	encodingConfig := argus.MakeTestEncodingConfig()
	ac := cmd.AppCreator{EncCfg: encodingConfig}
	serverCtx := server.NewDefaultContext()
	serverCtx.Config.Genesis = "someGenesis.json"
	clientCtx := client.Context{}
	serverCfg := serverconfig.DefaultConfig()
	serverCfg.MinGasPrices = "100stake"
	return argus.Start(serverCtx, clientCtx, serverCfg, ac.NewApp)
}
