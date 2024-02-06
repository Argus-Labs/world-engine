package receipt

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"pkg.world.dev/world-engine/relay/nakama/mocks"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
	"testing"
	"time"
)

func TestAddTxHashToPendingNotifications(t *testing.T) {
	logger := &testutils.NoOpLogger{}
	nk := mocks.NewNakamaModule(t)
	rd := NewReceiptsDispatcher()
	notifier := NewNotifier(logger, nk, rd)

	txHash := "tx123"
	userID := "user456"

	notifier.AddTxHashToPendingNotifications(txHash, userID)

	info, exists := notifier.txHashToTargetInfo[txHash]
	assert.True(t, exists, "TxHash should exist in the map after being added")
	assert.Equal(t, userID, info.userID, "UserID associated with TxHash does not match")
}

func TestHandleReceipt(t *testing.T) {
	logger := &testutils.NoOpLogger{}
	nk := mocks.NewNakamaModule(t)
	rd := NewReceiptsDispatcher()
	notifier := NewNotifier(logger, nk, rd)

	txHash := "tx123"
	userID := "user456"

	// Assert that "NotificationSend" is called with the given params
	nk.On("NotificationSend",
		mock.Anything, userID, "subject",
		mock.AnythingOfType("map[string]interface {}"), 1, "", false).Run(func(args mock.Arguments) {
		argData := args.Get(3).(map[string]any) //nolint:errcheck // [not important]
		assert.Nil(t, argData["result"])
		assert.Nil(t, argData["errors"])
	}).Return(nil).Once()

	notifier.txHashToTargetInfo[txHash] = targetInfo{
		createdAt: time.Now(),
		userID:    userID,
	}

	receipt := &Receipt{TxHash: txHash}
	err := notifier.handleReceipt(receipt)
	assert.NoError(t, err, "Handling receipt should not error")

	_, exists := notifier.txHashToTargetInfo[txHash]
	assert.False(t, exists, "TxHash should be removed from map after processing")
}

func TestCleanupStaleTransactions(t *testing.T) {
	logger := &testutils.NoOpLogger{}
	nk := mocks.NewNakamaModule(t)
	rd := NewReceiptsDispatcher()
	notifier := NewNotifier(logger, nk, rd)

	staleTxHash := "staleTx123"
	recentTxHash := "recentTx123"
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
