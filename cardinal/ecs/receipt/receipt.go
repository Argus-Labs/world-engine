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

type History struct {
	currTick     *atomic.Uint64
	ticksToStore uint64
	// Receipts for a given tick are assigned to an index into this history slice which acts as a ring buffer.
	history []map[transaction.TxID]Receipt
}

type Receipt struct {
	ID    transaction.TxID
	Value any
	Errs  []error
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
	for i := 0; i < ticksToStore; i++ {
		h.history = append(h.history, map[transaction.TxID]Receipt{})
	}
	h.currTick.Store(currentTick)
	return h
}

func (h *History) NextTick() {
	newCurr := h.currTick.Add(1)
	mod := newCurr % h.ticksToStore
	h.history[mod] = map[transaction.TxID]Receipt{}
}

func (h *History) AddError(id transaction.TxID, err error) {
	tick := int(h.currTick.Load() % h.ticksToStore)
	rec := h.history[tick][id]
	rec.ID = id
	rec.Errs = append(rec.Errs, err)
	h.history[tick][id] = rec
}

func (h *History) SetResult(id transaction.TxID, a any) {
	tick := int(h.currTick.Load() % h.ticksToStore)
	rec := h.history[tick][id]
	rec.ID = id
	rec.Value = a
	h.history[tick][id] = rec
}

func (h *History) GetReceipt(id transaction.TxID) (rec Receipt, ok bool) {
	tick := int(h.currTick.Load() % h.ticksToStore)
	rec, ok = h.history[tick][id]
	return rec, ok
}

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
