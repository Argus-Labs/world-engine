package test_utils

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/server"
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
	EventHub *events.WebSocketEventHub
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
