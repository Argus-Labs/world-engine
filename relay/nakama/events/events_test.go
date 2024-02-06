package events

import (
	"github.com/stretchr/testify/assert"
	"log"
	"net/http"
	"net/http/httptest"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{} // use default options

func setupMockWebSocketServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		defer c.Close()
		for {
			time.Sleep(time.Second) // Simulate some delay in sending messages, heh heh entropy go brrrrr
			err := c.WriteMessage(websocket.TextMessage, []byte(`{"message":"test event"}`))
			if err != nil {
				log.Println("write:", err)
				break
			}
		}
	}))
}

func TestEventHubIntegration(t *testing.T) {
	mockServer := setupMockWebSocketServer()
	defer mockServer.Close()

	logger := &testutils.NoOpLogger{}
	eventsEndpoint := "events"
	eventHub, err := CreateEventHub(logger, eventsEndpoint, strings.TrimPrefix(mockServer.URL, "http://"))
	if err != nil {
		t.Fatalf("Failed to create event hub: %v", err)
	}

	// Subscribe to the event hub
	session := "testSession"
	eventChan := eventHub.Subscribe(session)

	// Start dispatching events
	go func() {
		if err := eventHub.Dispatch(logger); err != nil {
			t.Logf("Error dispatching: %v", err)
		}
	}()

	// Wait to receive an event
	select {
	case event := <-eventChan:
		assert.True(t, strings.Contains(event.Message, "test event"))
	case <-time.After(5 * time.Second): // Adjust timeout as necessary
		t.Fatal("Did not receive event in time")
	}

	// Test unsubscribing
	eventHub.Unsubscribe(session)

	// Ensure channel is closed
	_, ok := <-eventChan
	assert.False(t, ok, "Channel should be closed after unsubscribe")

	// Cleanup and shutdown
	eventHub.Shutdown()
}
