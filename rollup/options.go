package rollup

import "github.com/argus-labs/argus/x/evm/types"

type AppOption func(*app)

func WithEVMHooks(h types.EvmHooks) AppOption {
	return func(a *app) {
		a.hooks = h
	}
}
