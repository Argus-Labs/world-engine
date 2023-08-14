package server

import "github.com/argus-labs/world-engine/cardinal/shard"

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
