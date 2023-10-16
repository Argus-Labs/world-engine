package ecs

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
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

func WithNamespace(ns string) Option {
	return func(w *World) {
		w.namespace = Namespace(ns)
	}
}

func WithPrettyLog() Option {
	return func(world *World) {
		prettyLogger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		world.Logger.Logger = &prettyLogger
	}
}

func WithStoreManager(s store.IManager) Option {
	return func(w *World) {
		w.storeManager = s
	}
}
