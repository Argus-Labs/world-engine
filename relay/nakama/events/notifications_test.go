package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"pkg.world.dev/world-engine/relay/nakama/mocks"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
)

const (
	eventsEndpoint = "events"
)

// Test that the Notifications system works as expected with the Dispatcher and a Mock Server.
func TestNotifierIntegrationWithEventHub(t *testing.T) {
	ch := make(chan TickResults, 1)
	nk := mocks.NewMockNakamaModule(t)
	logger := &testutils.FakeLogger{}
	mockServer := setupMockWebSocketServer(t, ch)
	eh, err := NewEventHub(logger, eventsEndpoint, strings.TrimPrefix(mockServer.URL, "http://"))
	if err != nil {
		t.Fatal("Failed to make new EventHub: ", err)
	}
	notifier := NewNotifier(logger, nk, eh)

	txHash := "hash1"
	userID := "user456"
	notifier.AddTxHashToPendingNotifications(txHash, userID)

	expectedNotifications := []*runtime.NotificationSend{
		{
			UserID:  userID,
			Subject: "receipt",
			Content: map[string]any{
				"txHash": txHash,
				"result": map[string]any{"status": "success"},
				"errors": []string{},
			},
			Code:       1,
			Sender:     "",
			Persistent: false,
		},
	}
	sendNotificationSuccessful := make(chan bool)
	nk.On("NotificationsSend", mock.Anything, expectedNotifications).
		Return(nil).
		Once().
		Run(func(mock.Arguments) {
			sendNotificationSuccessful <- true
		})

	dispatchErrCh := make(chan error)
	// Start dispatching events
	go func() {
		dispatchErrCh <- eh.Dispatch(logger)
	}()

	// Simulate Cardinal sending TickResults to the Nakama EventHub
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
	tr.Receipts = append(tr.Receipts, Receipt{
		TxHash: txHash,
		Result: map[string]any{"status": "success"},
		Errors: []string{},
	})
	ch <- tr

	select {
	case <-time.After(5 * time.Second):
		assert.Fail(t, "timeout while waiting for a notification to be sent")
	case <-sendNotificationSuccessful: // success
		break
	}

	eh.Shutdown()
	assert.NoError(t, <-dispatchErrCh)
}

func TestAddTxHashToPendingNotifications(t *testing.T) {
	ch := make(chan TickResults)
	logger := &testutils.FakeLogger{}
	nk := mocks.NewMockNakamaModule(t)
	mockServer := setupMockWebSocketServer(t, ch)
	eh, err := NewEventHub(logger, eventsEndpoint, strings.TrimPrefix(mockServer.URL, "http://"))
	if err != nil {
		t.Fatal("Failed to make new EventHub: ", err)
	}
	notifier := NewNotifier(logger, nk, eh)

	txHash := "hash1"
	userID := "user456"

	notifier.AddTxHashToPendingNotifications(txHash, userID)

	info, exists := notifier.txHashToTargetInfo[txHash]
	assert.True(t, exists, "TxHash should exist in the map after being added")
	assert.Equal(t, userID, info.userID, "UserID associated with TxHash does not match")
}

func TestHandleReceipt(t *testing.T) {
	ch := make(chan TickResults)
	logger := &testutils.FakeLogger{}
	nk := mocks.NewMockNakamaModule(t)
	mockServer := setupMockWebSocketServer(t, ch)
	eh, err := NewEventHub(logger, eventsEndpoint, strings.TrimPrefix(mockServer.URL, "http://"))
	if err != nil {
		t.Fatal("Failed to make new EventHub: ", err)
	}
	notifier := NewNotifier(logger, nk, eh)

	txHash := "hash1"
	userID := "user456"

	notifications := []*runtime.NotificationSend{
		{
			UserID:  userID,
			Subject: "receipt",
			Content: map[string]interface{}{
				"txHash": txHash,
				"result": (map[string]interface{})(nil),
				"errors": ([]string)(nil),
			},
			Code:       1,
			Sender:     "",
			Persistent: false,
		},
	}

	// Assert that "NotificationSend" is called with the given params
	nk.On("NotificationsSend", mock.Anything, notifications).Return(nil).Once()

	notifier.txHashToTargetInfo[txHash] = targetInfo{
		createdAt: time.Now(),
		userID:    userID,
	}

	receipt := []Receipt{{TxHash: txHash}}
	err = notifier.handleReceipt(receipt)
	require.NoError(t, err, "Handling receipt should not error")

	_, exists := notifier.txHashToTargetInfo[txHash]
	assert.False(t, exists, "TxHash should be removed from map after processing")
}

func TestCleanupStaleTransactions(t *testing.T) {
	ch := make(chan TickResults)
	logger := &testutils.FakeLogger{}
	nk := mocks.NewMockNakamaModule(t)
	mockServer := setupMockWebSocketServer(t, ch)
	eh, err := NewEventHub(logger, eventsEndpoint, strings.TrimPrefix(mockServer.URL, "http://"))
	if err != nil {
		t.Fatal("Failed to make new EventHub: ", err)
	}
	notifier := NewNotifier(logger, nk, eh)

	staleTxHash := "staleHash1"
	recentTxHash := "recentHash1"
	userID := "user456"

	// Add a stale transaction
	notifier.txHashToTargetInfo[staleTxHash] = targetInfo{
		createdAt: time.Now().Add(-2 * time.Hour), // 2 hours ago
		userID:    userID,
	}

	// Add a recent transaction
	notifier.txHashToTargetInfo[recentTxHash] = targetInfo{
		createdAt: time.Now(),
		userID:    userID,
	}

	notifier.cleanupStaleTransactions()

	_, staleExists := notifier.txHashToTargetInfo[staleTxHash]
	assert.False(t, staleExists, "Stale transaction should be removed")

	_, recentExists := notifier.txHashToTargetInfo[recentTxHash]
	assert.True(t, recentExists, "Recent transaction should not be removed")
}
