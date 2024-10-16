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

// DisableReplayProtection disables replay protection, so signed
// messages can be reused without being signed again, and they never expire.
// If replay protection is disabled, then WithMessageExpiration and
// WithHashCacheSize options have no effect
func DisableReplayProtection() Option {
	return func(s *Server) {
		s.config.isReplayProtectionDisabled = true
	}
}

// WithMessageExpiration How long messages will live past their creation
// time on the sender before they are considered to be expired and will
// not be processed. Default is 10 seconds.
// For longer expiration times you may also need to set a larger hash cache
// size using the WithHashCacheSize option
// NOTE: this means that the real time clock for the sender and receiver
// must be synchronized
func WithMessageExpiration(seconds int) Option {
	return func(s *Server) {
		s.config.messageExpirationSeconds = seconds
	}
}

// WithHashCacheSize how big the cache of hashes used for replay protection
// is allowed to be. Default is 1MB.
func WithHashCacheSize(sizeKB int) Option {
	return func(s *Server) {
		s.config.hashCacheSizeKB = sizeKB
	}
}
