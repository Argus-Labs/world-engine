// Package receipt keeps track of transaction receipts for a number of ticks. A receipt consists
// of any errors that were encountered while processing a transaction, as well as the transactions
// result.
package receipt

import (
	"errors"
	"sync/atomic"

	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
)

var (
	ErrorTickHasNotBeenProcessed = errors.New("tick is still in progress")
	ErrorOldTickHasBeenDiscarded = errors.New("the requested tick has been discarded due to age")
)

// History keeps track of transaction "receipts" (the result of a transaction and any associated errors) for some number
// of ticks.
type History struct {
	currTick     *atomic.Uint64
	ticksToStore uint64
	// Receipts for a given tick are assigned to an index into this history slice which acts as a ring buffer.
	history []map[transaction.TxID]Receipt
}

// Receipt contains a transaction ID, an arbitrary result, and a list of errors.
type Receipt struct {
	ID     transaction.TxID
	Result any
	Errs   []error
}

// NewHistory creates a object that can track transaction receipts over a number of ticks.
func NewHistory(currentTick uint64, ticksToStore int) *History {
	// Add an extra tick for the "current" tick.
	ticksToStore++
	h := &History{
		currTick: &atomic.Uint64{},
		// Store ticksToStore plus the "current" tick
		ticksToStore: uint64(ticksToStore),
	}
	h.history = make([]map[transaction.TxID]Receipt, 0, ticksToStore)
	for i := 0; i < ticksToStore; i++ {
		h.history = append(h.history, map[transaction.TxID]Receipt{})
	}
	h.currTick.Store(currentTick)
	return h
}

// NextTick advances the internal History tick by 1. Errors and results can only be set on the current tick. Receipts
// from ticks in the past are read only.
func (h *History) NextTick() {
	newCurr := h.currTick.Add(1)
	mod := newCurr % h.ticksToStore
	h.history[mod] = map[transaction.TxID]Receipt{}
}

// AddError associates the given error with the given transaction ID. Calling this multiple times will append
// the error any previously added errors.
func (h *History) AddError(id transaction.TxID, err error) {
	tick := int(h.currTick.Load() % h.ticksToStore)
	rec := h.history[tick][id]
	rec.ID = id
	rec.Errs = append(rec.Errs, err)
	h.history[tick][id] = rec
}

// SetResult sets the given transaction ID to the given result. Calling this multiple times will replace any previous
// results.
func (h *History) SetResult(id transaction.TxID, result any) {
	tick := int(h.currTick.Load() % h.ticksToStore)
	rec := h.history[tick][id]
	rec.ID = id
	rec.Result = result
	h.history[tick][id] = rec
}

// GetReceipt gets the receipt (the transaction result and the list of errors) for the given transaction id in the
// current tick. To get receipts from previous ticks use GetReceiptsForTick.
func (h *History) GetReceipt(id transaction.TxID) (rec Receipt, ok bool) {
	tick := int(h.currTick.Load() % h.ticksToStore)
	rec, ok = h.history[tick][id]
	return rec, ok
}

// GetReceiptsForTick gets all receipts for the given tick. If the tick is still active, or if the tick is too
// far in the past, an error is returned.
func (h *History) GetReceiptsForTick(tick uint64) ([]Receipt, error) {
	currTick := h.currTick.Load()
	// The requested tick is either in the future, or it is currently being processed. We don't yet know
	// what the results of this tick will be.
	if currTick <= tick {
		return nil, ErrorTickHasNotBeenProcessed
	}
	if currTick-tick >= h.ticksToStore {
		return nil, ErrorOldTickHasBeenDiscarded
	}
	mod := tick % h.ticksToStore
	recs := make([]Receipt, 0, len(h.history[mod]))
	for _, rec := range h.history[mod] {
		recs = append(recs, rec)
	}

	return recs, nil
}
