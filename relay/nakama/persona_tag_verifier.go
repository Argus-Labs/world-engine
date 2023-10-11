package main

import (
	"context"
	"fmt"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
)

// personaTagVerifier is a helper struct that asynchronously collects both persona tag registration requests (from
// nakama) AND persona tag transaction receipts from cardinal. When the result of both systems has been recorded,
// this struct attempts to update the user's PersonaTagStorageObj to reflect the success/failure of the claim persona
// tag request.
type personaTagVerifier struct {
	// txHashToPending keeps track of the state of pending claim persona tag requests. A sync.Map is not required
	// because all map updates happen in a single goroutine. Updates are transmitted to the goroutine
	// via the receiptCh channel and the pendingCh channel.
	txHashToPending map[string]pendingPersonaTagRequest
	receiptCh       receiptChan
	pendingCh       chan txHashAndUserID
	nk              runtime.NakamaModule
	logger          runtime.Logger
}

type pendingPersonaTagRequest struct {
	lastUpdate time.Time
	userID     string
	status     personaTagStatus
}

type txHashAndUserID struct {
	txHash string
	userID string
}

const personaVerifierSessionName = "persona_verifier_session"

func (p *personaTagVerifier) addPendingPersonaTag(userID, txHash string) {
	p.pendingCh <- txHashAndUserID{
		userID: userID,
		txHash: txHash,
	}
}

func initPersonaTagVerifier(logger runtime.Logger, nk runtime.NakamaModule, rd *receiptsDispatcher) (*personaTagVerifier, error) {
	ptv := &personaTagVerifier{
		txHashToPending: map[string]pendingPersonaTagRequest{},
		receiptCh:       make(receiptChan, 100),
		pendingCh:       make(chan txHashAndUserID),
		nk:              nk,
		logger:          logger,
	}
	rd.subscribe(personaVerifierSessionName, ptv.receiptCh)
	go ptv.consume()
	return ptv, nil
}

func (p *personaTagVerifier) consume() {
	cleanupTick := time.Tick(time.Minute)
	for {
		var currTxHash string
		select {
		case now := <-cleanupTick:
			p.cleanupStaleEntries(now)
		case receipt := <-p.receiptCh:
			currTxHash = p.handleReceipt(receipt)
		case pending := <-p.pendingCh:
			currTxHash = p.handlePending(pending)
		}
		if currTxHash == "" {
			continue
		}
		if err := p.attemptVerification(currTxHash); err != nil {
			p.logger.Error("failed to verify persona tag: %v", err)
		}
	}
}

func (p *personaTagVerifier) cleanupStaleEntries(now time.Time) {
	for key, val := range p.txHashToPending {
		if diff := now.Sub(val.lastUpdate); diff > time.Minute {
			delete(p.txHashToPending, key)
		}
	}
}

func (p *personaTagVerifier) handleReceipt(receipt *Receipt) string {
	result, ok := receipt.Result["Success"]
	if !ok {
		return ""
	}
	success, ok := result.(bool)
	if !ok {
		return ""
	}
	pending := p.txHashToPending[receipt.TxHash]
	pending.lastUpdate = time.Now()
	if success {
		pending.status = personaTagStatusAccepted
	} else {
		pending.status = personaTagStatusRejected
	}
	p.txHashToPending[receipt.TxHash] = pending
	return receipt.TxHash
}

func (p *personaTagVerifier) handlePending(tuple txHashAndUserID) string {
	pending := p.txHashToPending[tuple.txHash]
	pending.lastUpdate = time.Now()
	pending.userID = tuple.userID
	p.txHashToPending[tuple.txHash] = pending
	return tuple.txHash
}

func (p *personaTagVerifier) attemptVerification(txHash string) error {
	pending, ok := p.txHashToPending[txHash]
	if !ok || pending.userID == "" || pending.status == "" {
		// We're missing a success/failure message from cardinal or the initial request from the
		// user to claim a persona tag.
		return nil
	}
	// We have both a user ID and a success message. Save this success/failure to nakama's storage system
	ctx := context.Background()
	ctx = context.WithValue(ctx, runtime.RUNTIME_CTX_USER_ID, pending.userID)
	ptr, err := loadPersonaTagStorageObj(ctx, p.nk)
	if err != nil {
		return fmt.Errorf("unable to get persona tag storage obj: %w", err)
	}
	if ptr.Status != personaTagStatusPending {
		return fmt.Errorf("expected a pending persona tag status but got %q", ptr.Status)
	}
	ptr.Status = pending.status
	if err := ptr.savePersonaTagStorageObj(ctx, p.nk); err != nil {
		return fmt.Errorf("unable to set persona tag storage object: %w", err)
	}
	delete(p.txHashToPending, txHash)
	p.logger.Debug("result of associating user %q with persona tag %q: %v", pending.userID, ptr.PersonaTag, pending.status)
	return nil
}
