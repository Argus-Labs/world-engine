// Package receipt keeps track of transaction receipts for a number of ticks. A receipt consists
// of any errors that were encountered while processing a transaction, as well as the transactions
// result.
package receipt

import (
	"errors"
	"sync/atomic"

	"pkg.world.dev/world-engine/cardinal/interfaces"
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
	history []map[interfaces.TxHash]Receipt
}

// Receipt contains a transaction hash, an arbitrary result, and a list of errors.
type Receipt struct {
	TxHash interfaces.TxHash
	Result any
	Errs   []error
}

func (r *Receipt) GetTxHash() interfaces.TxHash {
	return r.TxHash
}

func (r *Receipt) GetResult() any {
	return r.Result
}

func (r *Receipt) GetErrors() []error {
	return r.Errs
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
	h.history = make([]map[interfaces.TxHash]Receipt, 0, ticksToStore)
	for i := 0; i < ticksToStore; i++ {
		h.history = append(h.history, map[interfaces.TxHash]Receipt{})
	}
	h.currTick.Store(currentTick)
	return h
}

func (h *History) Size() uint64 {
	return h.ticksToStore
}

// NextTick advances the internal History tick by 1. Errors and results can only be set on the current tick. Receipts
// from ticks in the past are read only.
func (h *History) NextTick() {
	newCurr := h.currTick.Add(1)
	mod := newCurr % h.ticksToStore
	h.history[mod] = map[interfaces.TxHash]Receipt{}
}

func (h *History) SetTick(tick uint64) {
	h.currTick.Store(tick)
}

// AddError associates the given error with the given transaction hash. Calling this multiple times will append
// the error any previously added errors.
func (h *History) AddError(hash interfaces.TxHash, err error) {
	tick := int(h.currTick.Load() % h.ticksToStore)
	rec := h.history[tick][hash]
	rec.TxHash = hash
	rec.Errs = append(rec.Errs, err)
	h.history[tick][hash] = rec
}

// SetResult sets the given transaction hash to the given result. Calling this multiple times will replace any previous
// results.
func (h *History) SetResult(hash interfaces.TxHash, result any) {
	tick := int(h.currTick.Load() % h.ticksToStore)
	rec := h.history[tick][hash]
	rec.TxHash = hash
	rec.Result = result
	h.history[tick][hash] = rec
}

// GetReceipt gets the receipt (the transaction result and the list of errors) for the given transaction hash in the
// current tick. To get receipts from previous ticks use GetReceiptsForTick.
func (h *History) GetReceipt(hash interfaces.TxHash) (rec interfaces.IReceipt, ok bool) {
	tick := int(h.currTick.Load() % h.ticksToStore)
	temp, ok := h.history[tick][hash]
	rec = &temp
	return rec, ok
}

// GetReceiptsForTick gets all receipts for the given tick. If the tick is still active, or if the tick is too
// far in the past, an error is returned.
func (h *History) GetReceiptsForTick(tick uint64) ([]interfaces.IReceipt, error) {
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
	recs := make([]interfaces.IReceipt, 0, len(h.history[mod]))
	for key := range h.history[mod] {
		value, ok := h.history[mod][key]
		if !ok {
			return nil, errors.New("Error key not found")
		}
		recs = append(recs, &value)
	}

	return recs, nil
}
