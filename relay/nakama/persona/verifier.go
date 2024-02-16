package persona

import (
	"context"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"time"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

// Verifier is a helper struct that asynchronously collects both persona tag registration requests (from
// nakama) AND persona tag transaction receipts from cardinal. When the result of both systems has been recorded,
// this struct attempts to update the user's StorageObj to reflect the success/failure of the claim persona
// tag request.
type Verifier struct {
	// txHashToPending keeps track of the state of pending claim persona tag requests. A sync.Map is not required
	// because all map updates happen in a single goroutine. Updates are transmitted to the goroutine
	// via the receiptCh channel and the pendingCh channel.
	txHashToPending map[string]pendingRequest
	receiptCh       chan []*receipt.Receipt
	pendingCh       chan txHashAndUserID
	nk              runtime.NakamaModule
	logger          runtime.Logger
}

type pendingRequest struct {
	lastUpdate time.Time
	userID     string
	status     personaTagStatus
}

type txHashAndUserID struct {
	txHash string
	userID string
}

const personaVerifierSessionName = "persona_verifier"

func (p *Verifier) AddPendingPersonaTag(userID, txHash string) {
	p.pendingCh <- txHashAndUserID{
		userID: userID,
		txHash: txHash,
	}
}

func NewVerifier(logger runtime.Logger, nk runtime.NakamaModule, rd *receipt.Dispatcher,
) *Verifier {
	//channelLimit := 100
	ptv := &Verifier{
		txHashToPending: map[string]pendingRequest{},
		receiptCh:       make(chan []*receipt.Receipt),
		pendingCh:       make(chan txHashAndUserID),
		nk:              nk,
		logger:          logger,
	}
	rd.Subscribe(personaVerifierSessionName, ptv.receiptCh)
	go ptv.consume()
	return ptv
}

func (p *Verifier) consume() {
	cleanupTick := time.NewTicker(time.Minute)
	for {
		var currTxHash []string
		select {
		case now := <-cleanupTick.C:
			p.cleanupStaleEntries(now)
		case receipts := <-p.receiptCh:
			currTxHash = p.handleReceipt(receipts)
		case pending := <-p.pendingCh:
			currTxHash = p.handlePending(pending)
		}
		if len(currTxHash) == 0 {
			continue
		}
		if err := p.attemptVerification(currTxHash); err != nil {
			p.logger.Error("failed to verify persona tag: %s", eris.ToString(err, true))
		}
	}
}

func (p *Verifier) cleanupStaleEntries(now time.Time) {
	for key, val := range p.txHashToPending {
		if diff := now.Sub(val.lastUpdate); diff > time.Minute {
			delete(p.txHashToPending, key)
		}
	}
}

func (p *Verifier) handleReceipt(receipts []*receipt.Receipt) []string {
	var hashes []string
	for _, rec := range receipts {
		// Note: receiptConstant is the key returned in the result
		// of the CreatePersonaResponse from Cardinal
		result, ok := rec.Result[receiptConstant]
		if !ok {
			// Receipts that do not have the "success" key will be discarded here
			continue
		}
		success, ok := result.(bool)
		if !ok {
			continue
		}
		pending := p.txHashToPending[rec.TxHash]
		pending.lastUpdate = time.Now()
		if success {
			pending.status = StatusAccepted
		} else {
			pending.status = StatusRejected
		}
		p.txHashToPending[rec.TxHash] = pending
		hashes = append(hashes, rec.TxHash)
	}
	return hashes
}

func (p *Verifier) handlePending(tuple txHashAndUserID) []string {
	pending := p.txHashToPending[tuple.txHash]
	pending.lastUpdate = time.Now()
	pending.userID = tuple.userID
	p.txHashToPending[tuple.txHash] = pending
	return []string{tuple.txHash}
}

func (p *Verifier) attemptVerification(txHashes []string) error {
	for _, txHash := range txHashes {
		pending, ok := p.txHashToPending[txHash]
		if !ok || pending.userID == "" || pending.status == "" {
			// We're missing a success/failure message from cardinal or the initial request from the
			// user to claim a persona tag.
			return nil
		}
		// We have both a user ID and a success message. Save this success/failure to nakama's storage system
		ctx := context.Background()
		ctx = context.WithValue(ctx, runtime.RUNTIME_CTX_USER_ID, pending.userID) //nolint:staticcheck // its fine.
		ptr, err := LoadPersonaTagStorageObj(ctx, p.nk)
		if err != nil {
			return eris.Wrap(err, "unable to get persona tag storage obj")
		}
		if ptr.Status != StatusPending {
			return eris.Errorf("expected a pending persona tag status but got %q", ptr.Status)
		}
		ptr.Status = pending.status
		if err = ptr.SavePersonaTagStorageObj(ctx, p.nk); err != nil {
			return eris.Wrap(err, "unable to set persona tag storage object")
		}
		delete(p.txHashToPending, txHash)
		p.logger.Debug("result of associating user %q with persona tag %q: %v", pending.userID, ptr.PersonaTag, pending.status)
	}

	return nil
}
