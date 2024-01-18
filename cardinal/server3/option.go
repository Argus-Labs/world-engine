package server3

import (
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"strconv"
)

type Option func(s *Server)

func WithPort(port uint) Option {
	return func(s *Server) {
		s.port = strconv.Itoa(int(port))
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

func WithDisableSwagger() Option {
	return func(s *Server) {
		s.disableSwagger = true
	}
}
