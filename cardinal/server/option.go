package server

type Option func(s *Server)

// WithPort allows the server to run on a specified port.
func WithPort(port string) Option {
	return func(s *Server) {
		s.config.port = port
	}
}

// DisableSwagger disables the Swagger setup of the server.
func DisableSwagger() Option {
	return func(s *Server) {
		s.config.isSwaggerDisabled = true
	}
}

// DisableSignatureVerification disables signature verification.
func DisableSignatureVerification() Option {
	return func(s *Server) {
		s.config.isSignatureValidationDisabled = true
	}
}

// must be synchronized.
func WithMessageExpiration(seconds uint) Option {
	return func(s *Server) {
		s.config.messageExpirationSeconds = seconds
	}
}

// This setting is ignored if the DisableSignatureVerification option is used.
func WithHashCacheSize(sizeKB uint) Option {
	return func(s *Server) {
		s.config.messageHashCacheSizeKB = sizeKB
	}
}
