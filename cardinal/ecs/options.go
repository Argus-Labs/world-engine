package ecs

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/shard"
)

type Option func(e *Engine)

func WithAdapter(adapter shard.Adapter) Option {
	return func(e *Engine) {
		e.chain = adapter
	}
}

func WithReceiptHistorySize(size int) Option {
	return func(e *Engine) {
		e.receiptHistory = receipt.NewHistory(e.CurrentTick(), size)
	}
}

func WithPrettyLog() Option {
	return func(engine *Engine) {
		prettyLogger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		engine.Logger = &prettyLogger
	}
}

func WithStoreManager(s store.IManager) Option {
	return func(e *Engine) {
		e.entityStore = s
	}
}

func WithEventHub(eventHub events.EventHub) Option {
	return func(e *Engine) {
		e.eventHub = eventHub
	}
}

func WithLoggingEventHub(logger *zerolog.Logger) Option {
	return func(e *Engine) {
		e.eventHub = events.NewLoggingEventHub(logger)
	}
}
