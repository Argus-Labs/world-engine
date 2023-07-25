package server

type Option func(th *Handler)

func DisableSignatureVerification() Option {
	return func(th *Handler) {
		th.disableSigVerification = true
	}
}

func WithPort(p string) Option {
	return func(th *Handler) {
		th.port = p
	}
}
