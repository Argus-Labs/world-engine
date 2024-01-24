package server

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
)

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

func WithPrettyPrint() Option {
	return func(_ *Server) {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

// DisableSwagger allows to disable the swagger setup of the server.
func DisableSwagger() Option {
	return func(s *Server) {
		s.config.isSwaggerDisabled = true
	}
}
