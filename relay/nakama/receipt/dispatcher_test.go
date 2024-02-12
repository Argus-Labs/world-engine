package receipt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
)

// Setup mock server in a more streamlined manner, directly using the URL provided by httptest.
func setupMockServer(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := TransactionReceiptsReply{
			StartTick: 0,
			EndTick:   1,
			Receipts:  []*Receipt{{TxHash: "hash1", Result: map[string]any{"status": "success"}, Errors: []string{}}},
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			return
		}
	}))

	t.Cleanup(server.Close)
	return server
}

// Test that the Dispatcher can poll receipts from the Server and pass them to subscribed channels.
func TestPollingFetchesAndDispatchesReceipts(t *testing.T) {
	dispatcher := NewDispatcher()
	mockServer := setupMockServer(t)

	testChannel := make(chan *Receipt, 10)
	dispatcher.Subscribe("testSessionPolling", testChannel)

	noOpLogger := &testutils.FakeLogger{}
	go dispatcher.Dispatch(noOpLogger)
	go dispatcher.PollReceipts(noOpLogger, strings.TrimPrefix(mockServer.URL, "http://"))

	select {
	case receivedReceipt := <-testChannel:
		assert.NotNil(t, receivedReceipt)
		assert.Equal(t, "hash1", receivedReceipt.TxHash)
	case <-time.After(time.Second):
		t.Fatal("Did not receive any receipts within the expected time")
	}
}

// Test error handling in the polling process.
func TestPollingHandlesErrorsGracefully(t *testing.T) {
	dispatcher := NewDispatcher()

	errorMockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
		if err != nil {
			return
		}
	}))
	defer errorMockServer.Close()

	noOpLogger := &testutils.FakeLogger{}
	go dispatcher.PollReceipts(noOpLogger, strings.TrimPrefix(errorMockServer.URL, "http://"))

	time.Sleep(2 * time.Second) // Wait for logging

	errors := noOpLogger.GetErrors()
	assert.NotEmpty(t, errors)
	assert.Contains(t, errors[0], "internal server error")
}

// Test the non-blocking behavior of the Dispatch method.
func TestNonBlockingDispatch(t *testing.T) {
	dispatcher := NewDispatcher()
	fullChannel := make(chan *Receipt)
	dispatcher.Subscribe("testSessionFull", fullChannel)

	noOpLogger := &testutils.FakeLogger{}
	go dispatcher.Dispatch(noOpLogger)

	done := make(chan bool, 1)
	go func() {
		dispatcher.ch <- &Receipt{TxHash: "blockTest"}
		done <- true
	}()

	select {
	case <-done:
		// Dispatch is non-blocking
	case <-time.After(2 * time.Second):
		t.Fatal("Dispatch should be non-blocking")
	}
}
