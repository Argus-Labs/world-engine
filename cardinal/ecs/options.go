package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/shard"
)

type Option func(w *World)

func WithAdapter(adapter shard.Adapter) Option {
	return func(w *World) {
		w.chain = adapter
	}
}

func WithReceiptHistorySize(size int) Option {
	return func(w *World) {
		w.receiptHistory = receipt.NewHistory(w.CurrentTick(), size)
	}
}

func WithNamespace(id string) Option {
	return func(w *World) {
		w.namespace = Namespace(id)
	}
}
