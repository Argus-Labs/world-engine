package rollup

import (
	argus "github.com/argus-labs/argus/app"
)

var _ Application = app{}

type app struct {
	evmHooks []EVMNakamaHook
	chain    argus.ArgusApp
}

// Start does starting things
//
// TODO(technicallyty): implement
func (a app) Start() error {
	return nil
}
