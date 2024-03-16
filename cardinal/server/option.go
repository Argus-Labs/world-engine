package server

type Option func(s *Server)

// WithPort allows the server to run on a specified port.
func WithPort(port string) Option {
	return func(s *Server) {
		s.config.port = port
	}
}

// DisableSignatureVerification disables signature verification.
func DisableSignatureVerification() Option {
	return func(s *Server) {
		s.config.isSignatureVerificationDisabled = true
	}
}

// DisableSwagger allows to disable the swagger setup of the server.
func DisableSwagger() Option {
	return func(s *Server) {
		s.config.isSwaggerDisabled = true
	}
}
