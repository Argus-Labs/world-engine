package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/go-openapi/runtime/middleware"
	"github.com/gofiber/contrib/swagger"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/shard"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrInvalidSignature is returned when a signature is incorrect in some way (e.g. namespace mismatch, nonce invalid,
	// the actual Verify fails). Other failures (e.g. Redis is down) should not wrap this error.
	ErrInvalidSignature = errors.New("invalid signature")
	path                string
)

const (
	gameQueryPrefix = "/query/game/"
	gameTxPrefix    = "/tx/game/"

	readHeaderTimeout = 5 * time.Second
)

func init() {
	file, err := os.CreateTemp("", "")
	if err != nil {
		panic("could not create temp file for swaggerFile")
	}
	_, err = file.Write(swaggerData)
	if err != nil {
		panic("could not write swaggerFile to temp file")
	}
	path = file.Name()
}

type Handler struct {
	w                      *ecs.Engine
	app                    *fiber.App
	disableSigVerification bool
	withCORS               bool
	running                atomic.Bool
	Port                   string
	shutdownMutex          sync.Mutex
	// Plugins
	adapter shard.WriteAdapter
}

func NewHandler(w *ecs.Engine, builder middleware.Builder, opts ...Option) (*Handler, error) {
	h, err := newHandlerEmbed(w, builder, opts...)
	h.running.Store(false)
	if err != nil {
		return nil, err
	}
	return h, nil
}

//go:embed swagger.yml
var swaggerData []byte

func newHandlerEmbed(w *ecs.Engine, builder middleware.Builder, opts ...Option) (*Handler, error) {
	handler := &Handler{
		w: w,
	}
	handler.Initialize()
	for _, opt := range opts {
		opt(handler)
	}

	// Setup swagger docs at /docs
	cfg := swagger.Config{
		FilePath: path,
		Title:    "World Engine API Docs",
	}
	handler.app.Use(swagger.New(cfg))

	// Register handlers
	err := handler.registerTxHandler()
	if err != nil {
		return nil, err
	}
	err = handler.registerQueryHandlers()
	if err != nil {
		return nil, err
	}
	handler.registerHealthHandler()
	handler.registerDebugHandler()

	return handler, nil
}

// EndpointsResult result struct for /query/http/endpoints.
type EndpointsResult struct {
	TxEndpoints    []string `json:"txEndpoints"`
	QueryEndpoints []string `json:"queryEndpoints"`
	DebugEndpoints []string `json:"debugEndpoints"`
}

func createAllEndpoints(world *ecs.Engine) (*EndpointsResult, error) {
	txs, err := world.ListMessages()
	if err != nil {
		return nil, err
	}
	txEndpoints := make([]string, 0, len(txs))
	for _, tx := range txs {
		if tx.Name() == ecs.CreatePersonaMsg.Name() {
			txEndpoints = append(txEndpoints, "/tx/persona/"+tx.Name())
		} else {
			txEndpoints = append(txEndpoints, gameTxPrefix+tx.Name())
		}
	}

	queries := world.ListQueries()
	queryEndpoints := make([]string, 0, len(queries))
	for _, query := range queries {
		queryEndpoints = append(queryEndpoints, gameQueryPrefix+query.Name())
	}
	queryEndpoints = append(queryEndpoints,
		"/query/http/endpoints",
		"/query/persona/signer",
		"/query/receipt/list",
		"/query/game/cql",
	)
	debugEndpoints := make([]string, 1)
	debugEndpoints[0] = "/debug/state"
	return &EndpointsResult{
		TxEndpoints:    txEndpoints,
		QueryEndpoints: queryEndpoints,
	}, nil
}

// Initialize initializes the app. It firsts checks for a port set on the handler via options.
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
	handler.app = fiber.New()
}

// Serve serves the application, blocking the calling thread.
// Call this in a new go routine to prevent blocking.
func (handler *Handler) Serve() error {
	hostname, err := os.Hostname()
	if err != nil {
		return eris.Wrap(err, "error getting hostname")
	}
	log.Info().Msgf("serving at %s:%s", hostname, handler.Port)
	handler.running.Store(true)
	err = handler.app.Listen(":" + handler.Port)
	if err != nil {
		return eris.Wrap(err, "error starting Fiber app")
	}
	handler.running.Store(false)
	return nil
}

func (handler *Handler) Shutdown() error {
	handler.shutdownMutex.Lock()
	defer handler.shutdownMutex.Unlock()
	if handler.running.Load() {
		log.Info().Msg("Shutting down app.")
		if err := handler.app.Shutdown(); err != nil {
			return eris.Wrap(err, "error shutting down Fiber app")
		}
		handler.running.Store(false)
		log.Info().Msg("Server successfully shutdown.")
	} else {
		log.Info().Msg("Server is not running or already shut down.")
	}
	return nil
}

func createQueryHandlerFromRequest[Request any, Response any](requestName string,
	requestHandler func(*Request) (*Response, error)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestBody := c.Body()

		var request Request
		if len(requestBody) != 0 {
			decoder := json.NewDecoder(bytes.NewReader(requestBody))
			decoder.DisallowUnknownFields()
			err := decoder.Decode(&request)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, eris.Wrapf(err, "unable to unmarshal query request into type %T", request).Error())
			}
		}
		resp, err := requestHandler(&request)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(resp)
	}
}
