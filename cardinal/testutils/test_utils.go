package testutils

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/ethereum/go-ethereum/crypto"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/sign"
)

func MakeTestTransactionHandler(t *testing.T, world *ecs.World, opts ...server.Option) *TestTransactionHandler {
	port := "4040"
	opts = append(opts, server.WithPort(port))
	eventHub := events.CreateWebSocketEventHub()
	world.SetEventHub(eventHub)
	eventBuilder := events.CreateNewWebSocketBuilder("/events", events.CreateWebSocketEventHandler(eventHub))
	txh, err := server.NewHandler(world, eventBuilder, opts...)
	assert.NilError(t, err)

	// add test websocket handler.
	txh.Mux.HandleFunc("/echo", events.Echo)

	healthPath := "/health"
	t.Cleanup(func() {
		assert.NilError(t, txh.Close())
	})

	go func() {
		err = txh.Serve()
		// ErrServerClosed is returned from txh.Serve after txh.Close is called. This is
		// normal.
		if !errors.Is(err, http.ErrServerClosed) {
			assert.NilError(t, err)
		}
	}()
	gameObject := server.NewGameManager(world, txh)
	t.Cleanup(func() {
		_ = gameObject.Shutdown()
	})

	host := "localhost:" + port
	healthURL := host + healthPath
	start := time.Now()
	for {
		assert.Check(t, time.Since(start) < time.Second, "timeout while waiting for a healthy server")
		//nolint:noctx,bodyclose // its for a test.
		resp, err := http.Get("http://" + healthURL)
		if err == nil && resp.StatusCode == 200 {
			// the health check endpoint was successfully queried.
			break
		}
	}

	return &TestTransactionHandler{
		Handler:  txh,
		T:        t,
		Host:     host,
		EventHub: eventHub,
	}
}

// TestTransactionHandler is a helper struct that can start an HTTP server on port 4040 with the given world.
type TestTransactionHandler struct {
	*server.Handler
	T        *testing.T
	Host     string
	EventHub events.EventHub
}

func (t *TestTransactionHandler) MakeHTTPURL(path string) string {
	return "http://" + t.Host + "/" + path
}

func (t *TestTransactionHandler) MakeWebSocketURL(path string) string {
	return "ws://" + t.Host + "/" + path
}

func (t *TestTransactionHandler) Post(path string, payload any) *http.Response {
	bz, err := json.Marshal(payload)
	assert.NilError(t.T, err)
	//nolint:noctx // its for a test its ok.
	res, err := http.Post(t.MakeHTTPURL(path), "application/json", bytes.NewReader(bz))
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
	// directly to the World queue. It's OK if the signature does not match the payload.
	sig, err := sign.NewTransaction(privateKey, name, "namespace", nonce, `{"some":"data"}`)
	if err != nil {
		panic(err)
	}
	return sig
}

func UniqueSignature() *sign.Transaction {
	return UniqueSignatureWithName("some-persona-tag")
}

func AddTransactionToWorldByAnyTransaction(
	world *cardinal.World,
	cardinalTx cardinal.AnyTransaction,
	value any,
	tx *sign.Transaction) {
	worldCtx := WorldToWorldContext(world)
	ecsWorld := cardinal.TestingWorldContextToECSWorld(worldCtx)

	txs, err := ecsWorld.ListTransactions()
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
		panic(fmt.Sprintf("cannot find transaction %q in registered transactions. Did you register it?",
			cardinalTx.Convert().Name()))
	}

	_, _ = ecsWorld.AddTransaction(txID, value, tx)
}

// MakeWorldAndTicker sets up a cardinal.World as well as a function that can execute one game tick. The *cardinal.World
// will be automatically started when doTick is called for the first time. The cardinal.World will be shut down at the
// end of the test. If doTick takes longer than 5 seconds to run, t.Fatal will be called.
func MakeWorldAndTicker(t *testing.T,
	opts ...cardinal.WorldOption) (
	world *cardinal.World,
	doTick func()) {
	startTickCh, doneTickCh := make(chan time.Time), make(chan uint64)
	opts = append(opts, cardinal.WithTickChannel(startTickCh), cardinal.WithTickDoneChannel(doneTickCh))
	world, err := cardinal.NewMockWorld(opts...)
	if err != nil {
		t.Fatalf("unable to make mock world: %v", err)
	}

	// Shutdown any world resources. This will be called whether the world has been started or not.
	t.Cleanup(func() {
		if err = world.ShutDown(); err != nil {
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
				case err = <-startupError:
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
