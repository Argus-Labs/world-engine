package rollup

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"

	argus "github.com/argus-labs/argus/app"
	"github.com/argus-labs/argus/cmd/argusd/cmd"
	"github.com/argus-labs/argus/x/evm/types"
)

var _ Application = app{}

func NewApplication(opts ...AppOption) Application {
	a := &app{}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

type app struct {
	hooks types.EvmHooks
}

// Start does starting things
//
// TODO(technicallyty): this is scrapped together, need a better configuration and setup stuff!
func (a app) Start() error {
	encodingConfig := argus.MakeTestEncodingConfig()
	ac := cmd.AppCreator{EncCfg: encodingConfig, EvmHooks: a.hooks}
	serverCtx := server.NewDefaultContext()
	serverCtx.Config.Genesis = "someGenesis.json" // TODO(technicallyty): this should come after config refactor WORLD-75
	clientCtx := client.Context{}
	serverCfg := serverconfig.DefaultConfig()
	serverCfg.MinGasPrices = "100stake" // TODO(technicallyty): this should come after config refactor WORLD-75
	return argus.Start(serverCtx, clientCtx, serverCfg, ac.NewApp)
}
