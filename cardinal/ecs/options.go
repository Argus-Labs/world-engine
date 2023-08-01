package ecs

import "github.com/argus-labs/world-engine/cardinal/shard"

type Option func(w *World)

func WithAdapter(adapter shard.Adapter) Option {
	return func(w *World) {
		w.chain = adapter
	}
}
