package ecs

import "github.com/argus-labs/world-engine/cardinal/chain"

type Option func(w *World)

func WithAdapter(adapter chain.Adapter) Option {
	return func(w *World) {
		w.chain = adapter
	}
}
