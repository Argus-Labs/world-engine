package server

import "pkg.world.dev/world-engine/cardinal/shard"

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
