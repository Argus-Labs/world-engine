package rollup

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"

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
	_ = ac
	serverCtx := server.NewDefaultContext()
	clientCtx := client.Context{}
	return argus.Start(serverCtx, clientCtx, ac.NewApp)
}
