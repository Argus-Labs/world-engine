package engine

import (
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/engine/receipt"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/shard"
)

type Option func(w *ecs.World)

func WithAdapter(adapter shard.Adapter) Option {
	return func(w *ecs.World) {
		w.chain = adapter
	}
}

func WithReceiptHistorySize(size int) Option {
	return func(w *ecs.World) {
		w.receiptHistory = receipt.NewHistory(w.CurrentTick(), size)
	}
}

func WithNamespace(ns string) Option {
	return func(w *ecs.World) {
		w.namespace = ecs.Namespace(ns)
	}
}

func WithPrettyLog() Option {
	return func(world *ecs.World) {
		prettyLogger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		world.Logger.Logger = &prettyLogger
	}
}
