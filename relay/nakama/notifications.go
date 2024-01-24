package main

import (
	"context"
	"pkg.world.dev/world-engine/relay/nakama/dispatcher"
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

// receiptNotifier is a struct that sends out notifications to users based on transaction receipts.
type receiptNotifier struct {
	// txHashToTargetInto maps a specific transaction hash to a user ID. A timestamp is also tracked so "stale" transaction
	// can be cleaned up.
	txHashToTargetInfo map[string]targetInfo
	// newTxHash is a channel that takes in txHash/userID tuples. An item on this channel signals to the receiptNotifier
	// that the given user ID must be informed about the given transaction.
	newTxHash chan txHashAndUser
	// staleDuration is how much time has to pass before an undelivered notification is treated as stale.
	staleDuration time.Duration

	// Nakama specific structs to log information and send transactions.
	nk     runtime.NakamaModule
	logger runtime.Logger
}

func newReceiptNotifier(logger runtime.Logger, nk runtime.NakamaModule) *receiptNotifier {
	rd := globalReceiptsDispatcher
	ch := make(chan *dispatcher.Receipt)
	rd.Subscribe("notifications", ch)
	notifier := &receiptNotifier{
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
func (r *receiptNotifier) AddTxHashToPendingNotifications(txHash string, userID string) {
	r.newTxHash <- txHashAndUser{
		txHash: txHash,
		userID: userID,
	}
}

// sendNotifications loops forever, consuming Receipts from the given channel and sending them to the relevant user.
func (r *receiptNotifier) sendNotifications(ch chan *dispatcher.Receipt) {
	ticker := time.Tick(r.staleDuration)

	for {
		select {
		case receipt := <-ch:
			if err := r.handleReceipt(receipt); err != nil {
				r.logger.Debug("failed to send receipt %v: %v", receipt, err)
			}
		case <-ticker:
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
func (r *receiptNotifier) handleReceipt(receipt *dispatcher.Receipt) error {
	ctx := context.Background()
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

	if err := r.nk.NotificationSend(ctx, target.userID, "subject", data, 1, "", false); err != nil {
		return eris.Wrapf(err, "unable to send tx hash %q to user %q", receipt.TxHash, target.userID)
	}
	return nil
}

// cleanupStaleTransactions identifies any transactions that have been pending for too long (see
// receiptNotifier.staleDuration) and deletes them.
func (r *receiptNotifier) cleanupStaleTransactions() {
	for txHash, info := range r.txHashToTargetInfo {
		if time.Since(info.createdAt) > r.staleDuration {
			delete(r.txHashToTargetInfo, txHash)
		}
	}
}
