package testutils

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"net/http"
	server "pkg.world.dev/world-engine/cardinal/server3"
	"sync"
	"testing"
	"time"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs"

	"pkg.world.dev/world-engine/assert"

	"github.com/ethereum/go-ethereum/crypto"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/sign"
)

func NewTestServer(
	t *testing.T,
	world *ecs.Engine,
	opts ...server.Option,
) *TestTransactionHandler {
	eventHub := events.NewWebSocketEventHub()
	world.SetEventHub(eventHub)
	//eventBuilder := events.CreateNewWebSocketBuilder(
	//	"/events",
	//	events.CreateWebSocketEventHandler(eventHub),
	//)
	srvr, err := server.New(world, opts...)
	assert.NilError(t, err)

	// add test websocket handler
	// TODO: Uncomment and fix
	//srvr.Mux.HandleFunc("/echo", events.Echo)

	healthPath := "/health"
	t.Cleanup(func() {
		srvr.Shutdown()
	})

	go func() {
		err = srvr.Serve()
		// ErrServerClosed is returned from srvr.Serve after srvr.Close is called. This is
		// normal.
		if !eris.Is(eris.Cause(err), http.ErrServerClosed) {
			assert.NilError(t, err)
		}
	}()

	host := "localhost:4040"
	healthURL := host + healthPath
	start := time.Now()
	for {
		if time.Since(start) > 3*time.Second {
			t.Fatal("timeout while waiting for healthy server")
		}
		//nolint:noctx,bodyclose // it's for a test.
		resp, err := http.Get("http://" + healthURL)
		if err == nil && resp.StatusCode == 200 {
			// the health check endpoint was successfully queried.
			break
		}
	}

	return &TestTransactionHandler{
		Server:   srvr,
		T:        t,
		Host:     host,
		EventHub: eventHub,
	}
}

// TestTransactionHandler is a helper struct that can start an HTTP server on port 4040 with the given world.
type TestTransactionHandler struct {
	*server.Server
	T        *testing.T
	Host     string
	EventHub events.EventHub
}

func (t *TestTransactionHandler) MakeHTTPURL(path string) string {
	if path[0] == '/' {
		path = path[1:]
	}
	return "http://" + t.Host + "/" + path
}

func (t *TestTransactionHandler) MakeWebSocketURL(path string) string {
	return "ws://" + t.Host + "/" + path
}

func (t *TestTransactionHandler) Post(path string, payload any) *http.Response {
	bz, err := json.Marshal(payload)
	assert.NilError(t.T, err)
	//nolint:noctx // its for a test its ok.
	url := t.MakeHTTPURL(path)
	log.Info().Msgf("sending post request to %s with payload %s", url, string(bz))
	res, err := http.Post(url, "application/json", bytes.NewReader(bz))
	assert.NilError(t.T, err)
	return res
}

func (t *TestTransactionHandler) Get(path string) *http.Response {
	url := t.MakeHTTPURL(path)
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	assert.NilError(t.T, err)
	res, err := http.DefaultClient.Do(req)
	assert.NilError(t.T, err)
	return res
}

func SetTestTimeout(t *testing.T, timeout time.Duration) {
	if _, ok := t.Deadline(); ok {
		// A deadline has already been set. Don't add an additional deadline.
		return
	}
	success := make(chan bool)
	t.Cleanup(func() {
		success <- true
	})
	go func() {
		select {
		case <-success:
			// test was successful. Do nothing
		case <-time.After(timeout):
			panic("test timed out")
		}
	}()
}

func WorldToWorldContext(world *cardinal.World) cardinal.WorldContext {
	return cardinal.TestingWorldToWorldContext(world)
}

var (
	nonce      uint64
	privateKey *ecdsa.PrivateKey
)

func UniqueSignatureWithName(name string) *sign.Transaction {
	if privateKey == nil {
		var err error
		privateKey, err = crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
	}
	nonce++
	// We only verify signatures when hitting the HTTP server, and in tests we're likely just adding transactions
	// directly to the Engine queue. It's OK if the signature does not match the payload.
	sig, err := sign.NewTransaction(privateKey, name, "namespace", nonce, `{"some":"data"}`)
	if err != nil {
		panic(err)
	}
	return sig
}

func UniqueSignature() *sign.Transaction {
	return UniqueSignatureWithName("some_persona_tag")
}

func AddTransactionToWorldByAnyTransaction(
	world *cardinal.World,
	cardinalTx cardinal.AnyMessage,
	value any,
	tx *sign.Transaction) {
	worldCtx := WorldToWorldContext(world)
	ecsWorld := cardinal.TestingWorldContextToECSWorld(worldCtx)

	txs, err := ecsWorld.ListMessages()
	if err != nil {
		panic(err)
	}
	txID := cardinalTx.Convert().ID()
	found := false
	for _, tx := range txs {
		if tx.ID() == txID {
			found = true
			break
		}
	}
	if !found {
		panic(
			fmt.Sprintf(
				"cannot find transaction %q in registered transactions. Did you register it?",
				cardinalTx.Convert().Name(),
			),
		)
	}

	_, _ = ecsWorld.AddTransaction(txID, value, tx)
}

// MakeWorldAndTicker sets up a cardinal.World as well as a function that can execute one game tick. The *cardinal.World
// will be automatically started when doTick is called for the first time. The cardinal.World will be shut down at the
// end of the test. If doTick takes longer than 5 seconds to run, t.Fatal will be called.
func MakeWorldAndTicker(
	t *testing.T,
	opts ...cardinal.WorldOption,
) (world *cardinal.World, doTick func()) {
	startTickCh, doneTickCh := make(chan time.Time), make(chan uint64)
	eventHub := events.NewWebSocketEventHub()
	opts = append(
		opts,
		cardinal.WithTickChannel(startTickCh),
		cardinal.WithTickDoneChannel(doneTickCh),
		cardinal.WithEventHub(eventHub),
	)
	world = NewTestWorld(t, opts...)

	// Shutdown any world resources. This will be called whether the world has been started or not.
	t.Cleanup(func() {
		if err := world.ShutDown(); err != nil {
			t.Fatalf("unable to shut down world: %v", err)
		}
	})

	startGameOnce := sync.Once{}
	// Create a function that will do a single game tick, making sure to start the game world the first time it is called.
	doTick = func() {
		timeout := time.After(5 * time.Second) //nolint:gomnd // fine for now.
		startGameOnce.Do(func() {
			startupError := make(chan error)
			go func() {
				// StartGame is meant to block forever, so any return value will be non-nil and cause for concern.
				// Also, calling t.Fatal from a non-main thread only reports a failure once the test on the main thread has
				// completed. By sending this error out on a channel we can fail the test right away (assuming doTick
				// has been called from the main thread).
				startupError <- world.StartGame()
			}()
			for !world.IsGameRunning() {
				select {
				case err := <-startupError:
					t.Fatalf("startup error: %v", err)
				case <-timeout:
					t.Fatal("timeout while waiting for game to start")
				default:
					time.Sleep(10 * time.Millisecond) //nolint:gomnd // its for testing its ok.
				}
			}
		})

		startTickCh <- time.Now()
		<-doneTickCh
	}

	return world, doTick
}
