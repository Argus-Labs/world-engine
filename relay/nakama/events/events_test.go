package events

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.world.dev/world-engine/relay/nakama/testutils"
)

var upgrader = websocket.Upgrader{} // use default options

func setupMockWebSocketServer(t *testing.T, ch chan TickResults) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		defer c.Close()
		for msg := range ch {
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

	// Start dispatching events
	go func() {
		if err := eventHub.Dispatch(logger); err != nil {
			t.Logf("Error dispatching: %v", err)
		}
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
		require.NoError(t, err)
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
}
