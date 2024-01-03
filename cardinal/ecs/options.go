package ecs

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/events"
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

func WithPrettyLog() Option {
	return func(world *World) {
		prettyLogger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		world.Logger.Logger = &prettyLogger
	}
}

func WithStoreManager(s store.IManager) Option {
	return func(w *World) {
		w.entityStore = s
	}
}

func WithEventHub(eventHub events.EventHub) Option {
	return func(w *World) {
		w.eventHub = eventHub
	}
}

func WithLoggingEventHub(logger *ecslog.Logger) Option {
	return func(w *World) {
		w.eventHub = events.NewLoggingEventHub(logger)
	}
}
