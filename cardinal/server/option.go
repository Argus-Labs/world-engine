package server

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/shard"
)

type Option func(th *Handler)

func DisableSignatureVerification() Option {
	return func(th *Handler) {
		th.disableSigVerification = true
	}
}

func WithAdapter(a shard.Adapter) Option {
	return func(th *Handler) {
		th.adapter = a
	}
}

func WithCORS() Option {
	return func(th *Handler) {
		th.withCORS = true
	}
}

func WithPrettyPrint() Option {
	return func(_ *Handler) {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}
