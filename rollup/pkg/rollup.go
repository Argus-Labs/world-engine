package pkg

import (
	argus "github.com/argus-labs/argus/app"
	"github.com/argus-labs/argus/cmd/argusd/cmd"
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
	cfg *Config
}

// Start starts the rollup.
func (a app) Start() error {
	cfg := a.cfg
	encodingConfig := argus.MakeTestEncodingConfig()
	ac := cmd.AppCreator{EncCfg: encodingConfig}
	return argus.Start(cfg.AppCfg, &cfg.ServerCtx, cfg.ClientCtx, &cfg.ServerCfg, cfg.Rollup, ac.NewApp)
}
