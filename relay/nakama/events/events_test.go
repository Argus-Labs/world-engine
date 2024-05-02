package events

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
)

var upgrader = websocket.Upgrader{} // use default options

var (
	// tickResultSentinel is a special tick result value that signals to the mock websocker server that the active
	// websocket should be closed. This doesn't prevent future requests from re-connecting to the websocket to consume
	// more data.
	closeWebSocketSentinel = TickResults{
		Tick: math.MaxUint64,
	}
)

func isCloseWebSocketSentinel(tr TickResults) bool {
	return tr.Tick == closeWebSocketSentinel.Tick
}

// setupMockWebSocketServer creates a test-only server that allows websocket connections. Any TickResults sent on the
// given channel will be pushed to the websocket. If the sentinel closeWebSocketSentinel TickResult arrives on the
// input channel, the websocket will be closed. Note, the server is still active, so another websocket connection can
// be made. The server is closed during the t.Cleanup phase of the test.
func setupMockWebSocketServer(t *testing.T, ch chan TickResults) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		defer c.Close()
		for msg := range ch {
			if isCloseWebSocketSentinel(msg) {
				assert.NilError(t, c.Close())
				break
			}
			data, err := json.Marshal(msg)
			if err != nil {
				log.Fatal("failed to marshal event")
			}
			err = c.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				log.Println("write:", err)
				break
			}
		}
	}))

	t.Cleanup(server.Close)
	return server
}

func TestEventHubIntegration(t *testing.T) {
	ch := make(chan TickResults)
	mockServer := setupMockWebSocketServer(t, ch)
	t.Cleanup(func() {
		mockServer.Close()
		close(ch)
	})

	logger := &testutils.FakeLogger{}
	eventHub, err := NewEventHub(logger, eventsEndpoint, strings.TrimPrefix(mockServer.URL, "http://"))
	if err != nil {
		t.Fatalf("Failed to create event hub: %v", err)
	}

	// Subscribe to the event hub
	session := "testSession"
	eventChan := eventHub.SubscribeToEvents(session)
	dispatchErrChan := make(chan error)

	// Start dispatching events
	go func() {
		dispatchErrChan <- eventHub.Dispatch(logger)
	}()

	// Simulate Cardinal sending TickResults to the Nakama EventHub
	go func() {
		event, err := json.Marshal(map[string]any{"message": "test event"})
		if err != nil {
			t.Error("failed to marshal map")
			return
		}
		tr := TickResults{
			Tick:     100,
			Receipts: nil,
			Events:   nil,
		}
		tr.Events = append(tr.Events, event)
		ch <- tr
	}()

	// Wait to receive an event
	select {
	case event := <-eventChan:
		jsonMap := make(map[string]any)
		err = json.Unmarshal(event, &jsonMap)
		assert.NilError(t, err)
		msg, ok2 := jsonMap["message"]
		assert.True(t, ok2)
		msgString, ok2 := msg.(string)
		assert.True(t, ok2)
		assert.True(t, strings.Contains(msgString, "test event"))
	case <-time.After(5 * time.Second):
		t.Fatal("Did not receive event in time")
	}

	eventHub.Unsubscribe(session)

	// Ensure channel is closed
	_, ok := <-eventChan
	assert.False(t, ok, "Channel should be closed after unsubscribe")

	// Cleanup and shutdown
	eventHub.Shutdown()

	assert.NilError(t, <-dispatchErrChan)
}

func TestEventHub_WhenWebSocketDisconnects_EventHubAutomaticallyReconnects(t *testing.T) {
	ch := make(chan TickResults, 1)
	mockServer := setupMockWebSocketServer(t, ch)
	t.Cleanup(func() {
		close(ch)
	})
	logger := &testutils.FakeLogger{}
	eventHub, err := NewEventHub(logger, eventsEndpoint, strings.TrimPrefix(mockServer.URL, "http://"))
	assert.NilError(t, err)

	session := "reconnectSession"
	eventChan := eventHub.SubscribeToEvents(session)

	dispatchErrChan := make(chan error)
	go func() {
		dispatchErrChan <- eventHub.Dispatch(logger)
	}()

	wantMsg := `{"message": "some-message"}`
	tickResults := TickResults{
		Tick:     100,
		Receipts: nil,
		Events: [][]byte{
			[]byte(wantMsg),
		},
	}

	var gotTickResults []byte

	// This tick result will be transmitted across the websocket
	ch <- tickResults

	// Make sure the tick results is pushed to the event channel in a reasonable amount of time
	select {
	case gotTickResults = <-eventChan:
		break
	case <-time.After(5 * time.Second):
		assert.Fail(t, "timeout while waiting for first tick result")
	}

	assert.Contains(t, string(gotTickResults), wantMsg)

	// Pushing this sentinel value to the channel will close the active websocket connection. EventHub should
	// attempt to reconnect in the background.
	ch <- closeWebSocketSentinel

	// This tick result will be transmitted across the websocket
	ch <- tickResults

	// Make sure the tick results is pushed to the event channel in a reasonable amount of time
	select {
	case gotTickResults = <-eventChan:
		break
	case <-time.After(5 * time.Second):
		assert.Fail(t, "timeout while waiting for first tick result")
	}

	assert.Contains(t, string(gotTickResults), wantMsg)

	// Cleanup and shutdown
	eventHub.Shutdown()

	assert.NilError(t, <-dispatchErrChan)
}
