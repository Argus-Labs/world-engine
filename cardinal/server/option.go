package server

import (
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
)

type Option func(s *Server)

func WithPort(port string) Option {
	return func(s *Server) {
		s.port = port
	}
}

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

func DisableSwagger() Option {
	return func(s *Server) {
		s.disableSwagger = true
	}
}
