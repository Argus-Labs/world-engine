package mocks

import (
	"fmt"
	"pkg.world.dev/world-engine/cardinal/router/iterator"
)

var _ iterator.Iterator = (*FakeIterator)(nil)

// FakeIterator mimics the behavior of a real transaction iterator for testing purposes.
type FakeIterator struct {
	objects []Iterable
}

type Iterable struct {
	Batches   []*iterator.TxBatch
	Tick      uint64
	Timestamp uint64
}

func NewFakeIterator(collection []Iterable) *FakeIterator {
	return &FakeIterator{
		objects: collection,
	}
}

// Each simulates iterating over transactions based on the provided ranges.
// It directly invokes the provided function with mock data for testing.
func (f *FakeIterator) Each(fn func(batch []*iterator.TxBatch, tick, timestamp uint64) error, ranges ...uint64) error {
	startTick := uint64(0)
	stopTick := uint64(0)
	if len(ranges) > 0 {
		startTick = ranges[0]
		if len(ranges) > 1 {
			stopTick = ranges[1]
			if startTick > stopTick {
				return fmt.Errorf("start tick %d is greater than stop tick %d", startTick, stopTick)
			}
		}
	}

	for _, val := range f.objects {
		// Skip this iteration if the tick is before the startTick or after the stopTick (if stopTick is specified).
		if val.Tick < startTick || (stopTick != 0 && val.Tick > stopTick) {
			continue
		}

		// Invoke the callback function with the current batch, tick, and timestamp.
		if err := fn(val.Batches, val.Tick, val.Timestamp); err != nil {
			return err
		}
	}

	return nil
}
