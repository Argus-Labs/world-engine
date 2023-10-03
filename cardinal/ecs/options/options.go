package options

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/public"
	"pkg.world.dev/world-engine/cardinal/shard"
)

type Option func(w public.IWorld)

func WithAdapter(adapter shard.Adapter) Option {
	return func(w public.IWorld) {
		w.SetChain(&adapter)
	}
}

func WithReceiptHistorySize(size int) Option {
	return func(w public.IWorld) {
		w.SetReceiptHistory(receipt.NewHistory(w.CurrentTick(), size))
	}
}

func WithNamespace(ns string) Option {
	return func(w public.IWorld) {
		w.SetNamespace(ns)
	}
}

func WithPrettyLog() Option {
	return func(world public.IWorld) {
		prettyLogger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		world.GetLogger().InjectLogger(&prettyLogger)
	}
}
