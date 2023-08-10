package ecs

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/receipt"
	"github.com/argus-labs/world-engine/cardinal/shard"
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

func WithWorldID(id string) Option {
	return func(w *World) {
		w.id = WorldId(id)
	}
}
