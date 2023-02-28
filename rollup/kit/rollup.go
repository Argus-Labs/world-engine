package kit

import (
	argus "github.com/argus-labs/argus/app"
	"github.com/argus-labs/argus/cmd/argusd/cmd"
	"github.com/argus-labs/argus/x/evm/types"
)

var _ Application = app{}

func NewApplication(cfg *Config, opts ...AppOption) Application {
	a := &app{cfg: cfg}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

type app struct {
	cfg   *Config
	hooks types.EvmHooks
}

// Start starts the rollup.
func (a app) Start() error {
	cfg := a.cfg
	encodingConfig := argus.MakeTestEncodingConfig()
	ac := cmd.AppCreator{EncCfg: encodingConfig, EvmHooks: a.hooks}
	return argus.Start(cfg.AppCfg, &cfg.ServerCtx, cfg.ClientCtx, &cfg.ServerCfg, cfg.Rollup, ac.NewApp)
}
