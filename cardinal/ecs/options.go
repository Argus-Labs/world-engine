package ecs

import (
	"os"
	"pkg.world.dev/world-engine/cardinal/router/adapter"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
)

type Option func(e *Engine)

func WithAdapter(adapter adapter.Adapter) Option {
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

func WithStoreManager(s gamestate.Manager) Option {
	return func(e *Engine) {
		e.entityStore = s
	}
}
