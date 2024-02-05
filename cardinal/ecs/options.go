package ecs

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
)

type Option func(e *Engine)

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
