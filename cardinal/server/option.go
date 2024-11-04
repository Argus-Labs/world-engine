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

// WithMessageExpiration How long messages will live past their creation
// time on the sender before they are considered to be expired and will
// not be processed. Default is 10 seconds.
// For longer expiration times you may also need to set a larger hash cache
// size using the WithHashCacheSize option
// This setting is ignored if the DisableSignatureVerification option is used
// NOTE: this means that the real time clock for the sender and receiver
// must be synchronized
func WithMessageExpiration(seconds uint) Option {
	return func(s *Server) {
		s.config.messageExpirationSeconds = seconds
	}
}

// WithHashCacheSize how big the cache of hashes used for replay protection
// is allowed to be. Default is 1MB.
// This setting is ignored if the DisableSignatureVerification option is used
func WithHashCacheSize(sizeKB uint) Option {
	return func(s *Server) {
		s.config.messageHashCacheSizeKB = sizeKB
	}
}
