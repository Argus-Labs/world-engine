package receipt

import (
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"pkg.world.dev/world-engine/relay/nakama/mocks"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
	"strings"
	"testing"
	"time"
)

// Test that the Notifications system works as expected with the Dispatcher and a Mock Server
func TestNotifierIntegrationWithDispatcher(t *testing.T) {
	nk := mocks.NewNakamaModule(t)
	logger := &testutils.FakeLogger{}
	mockServer := setupMockServer(t)
	rd := NewDispatcher()
	notifier := NewNotifier(logger, nk, rd)

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
			Persistent: true,
		},
	}
	nk.On("NotificationsSend", mock.Anything, expectedNotifications).Return(nil).Once()

	go rd.Dispatch(logger)
	go rd.PollReceipts(logger, strings.TrimPrefix(mockServer.URL, "http://"))

	time.Sleep(time.Second)
}

func TestAddTxHashToPendingNotifications(t *testing.T) {
	logger := &testutils.FakeLogger{}
	nk := mocks.NewNakamaModule(t)
	rd := NewDispatcher()
	notifier := NewNotifier(logger, nk, rd)

	txHash := "hash1"
	userID := "user456"

	notifier.AddTxHashToPendingNotifications(txHash, userID)

	info, exists := notifier.txHashToTargetInfo[txHash]
	assert.True(t, exists, "TxHash should exist in the map after being added")
	assert.Equal(t, userID, info.userID, "UserID associated with TxHash does not match")
}

func TestHandleReceipt(t *testing.T) {
	logger := &testutils.FakeLogger{}
	nk := mocks.NewNakamaModule(t)
	rd := NewDispatcher()
	notifier := NewNotifier(logger, nk, rd)

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
			Persistent: true,
		},
	}

	// Assert that "NotificationSend" is called with the given params
	nk.On("NotificationsSend", mock.Anything, notifications).Return(nil).Once()

	notifier.txHashToTargetInfo[txHash] = targetInfo{
		createdAt: time.Now(),
		userID:    userID,
	}

	receipt := []*Receipt{{TxHash: txHash}}
	err := notifier.handleReceipt(receipt)
	assert.NoError(t, err, "Handling receipt should not error")

	_, exists := notifier.txHashToTargetInfo[txHash]
	assert.False(t, exists, "TxHash should be removed from map after processing")
}

func TestCleanupStaleTransactions(t *testing.T) {
	logger := &testutils.FakeLogger{}
	nk := mocks.NewNakamaModule(t)
	rd := NewDispatcher()
	notifier := NewNotifier(logger, nk, rd)

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
