package server2

import (
	"errors"
	"github.com/go-openapi/runtime/middleware"
	"github.com/gofiber/contrib/swagger"
	"github.com/gofiber/fiber/v2"
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/shard"
	"strconv"
	"sync/atomic"
	"time"
)

var (
	// ErrInvalidSignature is returned when a signature is incorrect in some way (e.g. namespace mismatch, nonce invalid,
	// the actual Verify fails). Other failures (e.g. Redis is down) should not wrap this error.
	ErrInvalidSignature = errors.New("invalid signature")
)

const (
	gameQueryPrefix = "/query/game/"
	gameTxPrefix    = "/tx/game/"

	readHeaderTimeout = 5 * time.Second
)

type Handler struct {
	w                      *ecs.World
	server                 *fiber.App
	disableSigVerification bool
	withCORS               bool
	running                atomic.Bool
	Port                   string
	// Plugins
	adapter shard.WriteAdapter
}

func NewHandler(w *ecs.World, builder middleware.Builder, opts ...Option) (*Handler, error) {
	h, err := newHandlerEmbed(w, builder, opts...)
	h.running.Store(false)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func newHandlerEmbed(w *ecs.World, builder middleware.Builder, opts ...Option) (*Handler, error) {
	th := &Handler{
		w: w,
	}
	th.Initialize()
	for _, opt := range opts {
		opt(th)
	}
	// Default path to swagger docs is /docs
	cfg := swagger.Config{
		FilePath: "./swagger.yml",
		Title:    "World Engine API Docs",
	}
	th.server.Use(swagger.New(cfg))

	return th, nil
}

// Initialize initializes the server. It firsts checks for a port set on the handler via options.
// if no port is found, or a bad port was passed into the option, it falls back to an environment variable,
// CARDINAL_PORT. If not set, it falls back to a default port of 4040.
func (handler *Handler) Initialize() {
	if _, err := strconv.Atoi(handler.Port); err != nil || len(handler.Port) == 0 {
		envPort := os.Getenv("CARDINAL_PORT")
		if _, err = strconv.Atoi(envPort); err == nil {
			handler.Port = envPort
		} else {
			handler.Port = "4040"
		}
	}
	handler.server = fiber.New()
}
