package server

import (
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
)

type Option func(s *Server)

// WithPort allows the server to run on a specified port.
func WithPort(port string) Option {
	return func(s *Server) {
		s.port = port
	}
}

// DisableSignatureVerification disables signature verification.
func DisableSignatureVerification() Option {
	return func(th *Server) {
		th.disableSignatureVerification = true
	}
}

func WithCORS() Option {
	return func(th *Server) {
		th.app.Use(cors.New())
		th.withCORS = true
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
		s.disableSwagger = true
	}
}
