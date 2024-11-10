package world

import "github.com/rs/zerolog/log"

type Option func(*World)

func WithVerifySignature(verifySignature bool) Option {
	return func(w *World) {
		if !verifySignature {
			log.Warn().Msg("Signature verification is disabled. This is not recommended for production.")
		}
		w.config.CardinalVerifySignature = verifySignature
	}
}
