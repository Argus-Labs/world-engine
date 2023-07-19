package server

import "github.com/argus-labs/world-engine/cardinal/chain"

type Option func(th *Handler)

func DisableSignatureVerification() Option {
	return func(th *Handler) {
		th.disableSigVerification = true
	}
}

func WithAdapter(a chain.Adapter) Option {
	return func(th *Handler) {
		th.adapter = a
	}
}
