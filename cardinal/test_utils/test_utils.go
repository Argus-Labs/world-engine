package test_utils

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"gotest.tools/v3/assert"
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

	//add test websocket handler.
	txh.Mux.HandleFunc("/echo", events.Echo)

	healthPath := "/health"
	t.Cleanup(func() {
		assert.NilError(t, txh.Close())
	})

	go func() {
		err = txh.Serve()
		// ErrServerClosed is returned from txh.Serve after txh.Close is called. This is
		// normal.
		if err != http.ErrServerClosed {
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

func (t *TestTransactionHandler) MakeHttpURL(path string) string {
	return "http://" + t.Host + "/" + path
}

func (t *TestTransactionHandler) MakeWebSocketURL(path string) string {
	return "ws://" + t.Host + "/" + path
}

func (t *TestTransactionHandler) Post(path string, payload any) *http.Response {
	bz, err := json.Marshal(payload)
	assert.NilError(t.T, err)

	res, err := http.Post(t.MakeHttpURL(path), "application/json", bytes.NewReader(bz))
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
			//assert.Check(t, false, "test timed out")
			panic("test timed out")
		}
	}()
}

func WorldToWorldContext(world *cardinal.World) cardinal.WorldContext {
	var stolenContext cardinal.WorldContext
	world.Init(func(worldCtx cardinal.WorldContext) {
		stolenContext = worldCtx
	})
	return stolenContext
}

var (
	nonce      uint64
	privateKey *ecdsa.PrivateKey
)

func uniqueSignature() *sign.SignedPayload {
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
	sig, err := sign.NewSignedPayload(privateKey, "some-persona-tag", "namespace", nonce, `{"some":"data"}`)
	if err != nil {
		panic(err)
	}
	return sig
}

func AddTransactionToWorldByAnyTransaction(world *cardinal.World, cardinalTx cardinal.AnyTransaction, value any) {
	worldCtx := WorldToWorldContext(world)
	var ecsWorld *ecs.World

	// There are two options for converting a cardinal.World into an ecs.World.

	// Option A: Use a public method on the worldCtx object. This has the advantage that the "TestOnlyGetECSWorld"
	// method does NOT show up in the godoc, however the type assertion is convoluted.
	type HasTestOnlyGetECSWorld interface {
		TestOnlyGetECSWorld() *ecs.World
	}
	ecsWorld = worldCtx.(HasTestOnlyGetECSWorld).TestOnlyGetECSWorld()

	// Option B: Just make the conversion method a top level function. This method (and implementation details) are more
	// direct, however this means the "TestOnlyGetECSWorld" method will appear in the godoc.
	ecsWorld = cardinal.TestOnlyGetECSWorld(worldCtx)

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
		panic(fmt.Sprintf("cannot find transaction %q in registered transactinos. did you register it?", cardinalTx.Convert().Name()))
	}
	// uniqueSignature is copied from
	sig := uniqueSignature()
	_, _ = ecsWorld.AddTransaction(txID, value, sig)
}
