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

func setupMockWebSocketServer(t *testing.T, ch chan string) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		defer c.Close()
		for msg := range ch {
			err = c.WriteMessage(websocket.TextMessage, []byte(msg))
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
	ch := make(chan string)
	mockServer := setupMockWebSocketServer(t, ch)
	t.Cleanup(func() {
		mockServer.Close()
		close(ch)
	})

	logger := &testutils.FakeLogger{}
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

	// Simulate a WebSocket message by sending a message into the channel
	go func() {
		ch <- `{"message":"test event"}`
	}()

	// Wait to receive an event
	select {
	case event := <-eventChan:
		assert.True(t, strings.Contains(event.Message, "test event"))
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
