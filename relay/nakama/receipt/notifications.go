package receipt

import (
	"context"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

// targetInfo contains information about who should receive a notification. It contains a user ID as well as
// when this record of notification was created.
type targetInfo struct {
	createdAt time.Time
	userID    string
}

// txHashAndUser is a tuple of a transaction hash and a userID.
type txHashAndUser struct {
	txHash string
	userID string
}

// Notifier is a struct that sends out notifications to users based on transaction receipts.
type Notifier struct {
	// txHashToTargetInto maps a specific transaction hash to a user ID. A timestamp is also tracked so "stale" transaction
	// can be cleaned up.
	txHashToTargetInfo map[string]targetInfo
	// newTxHash is a channel that takes in txHash/userID tuples. An item on this channel signals to the ReceiptNotifier
	// that the given user ID must be informed about the given transaction.
	newTxHash chan txHashAndUser
	// staleDuration is how much time has to pass before an undelivered notification is treated as stale.
	staleDuration time.Duration

	// Nakama specific structs to log information and send transactions.
	nk     runtime.NakamaModule
	logger runtime.Logger
}

func NewNotifier(logger runtime.Logger, nk runtime.NakamaModule, rd *Dispatcher) *Notifier {
	ch := make(chan []*Receipt)
	rd.Subscribe("notifications", ch)
	notifier := &Notifier{
		txHashToTargetInfo: map[string]targetInfo{},
		nk:                 nk,
		logger:             logger,
		staleDuration:      time.Minute,
		newTxHash:          make(chan txHashAndUser),
	}

	go notifier.sendNotifications(ch)

	return notifier
}

// AddTxHashToPendingNotifications adds the given user ID and tx hash to pending notifications. When this system
// becomes aware of a transaction receipt with the given tx hash, the given user will be sent a notification with any
// results and errors.
// This method is safe for concurrent access.
func (r *Notifier) AddTxHashToPendingNotifications(txHash string, userID string) {
	r.newTxHash <- txHashAndUser{
		txHash: txHash,
		userID: userID,
	}
}

// sendNotifications loops forever, consuming Receipts from the given channel and sending them to the relevant user.
func (r *Notifier) sendNotifications(ch chan []*Receipt) {
	ticker := time.NewTicker(r.staleDuration)

	for {
		select {
		case receipts := <-ch:
			if err := r.handleReceipt(receipts); err != nil {
				r.logger.Debug("failed to send batch of receipts of len %d: %v", len(receipts), err)
			}
		case <-ticker.C:
			r.cleanupStaleTransactions()
		case tx := <-r.newTxHash:
			r.txHashToTargetInfo[tx.txHash] = targetInfo{
				createdAt: time.Now(),
				userID:    tx.userID,
			}
		}
	}
}

// handleReceipt identifies the relevant user for this receipt and sends them a notification.
func (r *Notifier) handleReceipt(receipts []*Receipt) error {
	ctx := context.Background()

	//nolint:prealloc // we cannot know how many notifications we're going to get
	var notifications []*runtime.NotificationSend
	for _, receipt := range receipts {
		target, ok := r.txHashToTargetInfo[receipt.TxHash]
		if !ok {
			return eris.Errorf("unable to find user for tx hash %q", receipt.TxHash)
		}
		delete(r.txHashToTargetInfo, receipt.TxHash)

		data := map[string]any{
			"txHash": receipt.TxHash,
			"result": receipt.Result,
			"errors": receipt.Errors,
		}

		notifications = append(notifications, &runtime.NotificationSend{
			UserID:     target.userID,
			Subject:    "receipt",
			Content:    data,
			Code:       1,
			Sender:     "",
			Persistent: false,
		})
	}

	if err := r.nk.NotificationsSend(ctx, notifications); err != nil {
		return eris.Wrapf(err, "unable to send batch of %d receipts from Nakama Notifier", len(receipts))
	}
	return nil
}

// cleanupStaleTransactions identifies any transactions that have been pending for too long (see
// ReceiptNotifier.staleDuration) and deletes them.
func (r *Notifier) cleanupStaleTransactions() {
	for txHash, info := range r.txHashToTargetInfo {
		if time.Since(info.createdAt) > r.staleDuration {
			delete(r.txHashToTargetInfo, txHash)
		}
	}
}
