package ecs

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/interfaces"
	"pkg.world.dev/world-engine/cardinal/shard"
)

type Option func(w interfaces.IWorld)

func WithAdapter(adapter shard.Adapter) Option {
	return func(w interfaces.IWorld) {
		w.SetChain(&adapter)
	}
}

func WithReceiptHistorySize(size int) Option {
	return func(w interfaces.IWorld) {
		w.SetReceiptHistory(receipt.NewHistory(w.CurrentTick(), size))
	}
}

func WithNamespace(ns string) Option {
	return func(w interfaces.IWorld) {
		w.SetNamespace(ns)
	}
}

func WithPrettyLog() Option {
	return func(world interfaces.IWorld) {
		prettyLogger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		world.GetLogger().InjectLogger(&prettyLogger)
	}
}
