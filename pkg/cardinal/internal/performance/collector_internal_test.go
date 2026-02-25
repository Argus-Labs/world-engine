package performance

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_BatchFlushesAfterNTicks(t *testing.T) {
	c := NewCollector(3)
	ch := c.Subscribe()

	now := time.Now()
	for i := range 3 {
		c.StartTick()
		c.RecordSpan(TickSpan{SystemName: "sys", TickHeight: uint64(i)})
		c.RecordTick(uint64(i), now)
		now = now.Add(50 * time.Millisecond)
	}

	select {
	case batch := <-ch:
		assert.Len(t, batch.Ticks, 3)
		assert.Equal(t, uint64(0), batch.Ticks[0].TickHeight)
		assert.Equal(t, uint64(2), batch.Ticks[2].TickHeight)
		assert.Len(t, batch.Ticks[0].Spans, 1)
	default:
		t.Fatal("expected a batch after 3 ticks")
	}
}

func TestCollector_NoBatchBeforeThreshold(t *testing.T) {
	c := NewCollector(5)
	ch := c.Subscribe()

	now := time.Now()
	for i := range 4 {
		c.StartTick()
		c.RecordTick(uint64(i), now)
	}

	select {
	case <-ch:
		t.Fatal("should not have received a batch before reaching batchSize")
	default:
	}
}

func TestCollector_MultipleSubscribers(t *testing.T) {
	c := NewCollector(1)
	ch1 := c.Subscribe()
	ch2 := c.Subscribe()

	now := time.Now()
	c.StartTick()
	c.RecordTick(0, now)

	batch1 := <-ch1
	batch2 := <-ch2
	assert.Len(t, batch1.Ticks, 1)
	assert.Len(t, batch2.Ticks, 1)
}

func TestCollector_Unsubscribe(t *testing.T) {
	c := NewCollector(1)
	ch := c.Subscribe()
	c.Unsubscribe(ch)

	now := time.Now()
	c.StartTick()
	c.RecordTick(0, now)

	select {
	case <-ch:
		t.Fatal("should not receive a batch after unsubscribe")
	default:
	}
}

func TestCollector_NonBlockingSend(t *testing.T) {
	c := NewCollector(1)
	ch := c.Subscribe()

	now := time.Now()
	for i := range subscriberChanBuf + 2 {
		c.StartTick()
		c.RecordTick(uint64(i), now)
	}

	received := 0
	for range subscriberChanBuf {
		select {
		case <-ch:
			received++
		default:
		}
	}
	assert.Equal(t, subscriberChanBuf, received)
}

func TestCollector_DroppedSpans(t *testing.T) {
	c := NewCollector(1)
	ch := c.Subscribe()

	now := time.Now()
	c.StartTick()
	for i := range defaultMaxSpansPerTick + 5 {
		c.RecordSpan(TickSpan{SystemName: "sys", TickHeight: 0, SystemHook: uint8(i % 4)})
	}
	c.RecordTick(0, now)

	batch := <-ch
	assert.Equal(t, uint64(5), batch.DroppedSpans, "batch should report 5 dropped spans")
	assert.Equal(t, uint64(0), c.DroppedSpans(), "counter should reset after flush")
}

func TestCollector_DroppedBatches(t *testing.T) {
	c := NewCollector(1)
	ch := c.Subscribe()

	now := time.Now()
	// Fill subscriber buffer (capacity = subscriberChanBuf = 4), then overflow.
	for i := range subscriberChanBuf + 2 {
		c.StartTick()
		c.RecordTick(uint64(i), now)
	}

	// Drain one batch to make room, then trigger another flush.
	<-ch
	c.StartTick()
	c.RecordTick(uint64(subscriberChanBuf+2), now)

	// Drain remaining batches until we find one with DroppedBatches > 0.
	var found bool
	for range subscriberChanBuf + 1 {
		select {
		case b := <-ch:
			if b.DroppedBatches > 0 {
				found = true
			}
		default:
		}
	}
	assert.True(t, found, "should report dropped batches for slow subscriber")
}

func TestCollector_Reset(t *testing.T) {
	c := NewCollector(10)
	ch := c.Subscribe()

	now := time.Now()
	c.StartTick()
	c.RecordSpan(TickSpan{SystemName: "sys"})
	c.RecordTick(0, now)

	c.StartTick()
	for range defaultMaxSpansPerTick + 1 {
		c.RecordSpan(TickSpan{SystemName: "sys"})
	}
	c.RecordTick(1, now)
	require.Positive(t, c.DroppedSpans())

	c.Reset()

	assert.Equal(t, uint64(0), c.DroppedSpans())

	select {
	case <-ch:
		t.Fatal("should not have received a batch after reset")
	default:
	}
}

func TestCollector_SpanWithoutStartTick(t *testing.T) {
	c := NewCollector(1)
	_ = c.Subscribe()

	c.RecordSpan(TickSpan{SystemName: "orphan"})
	assert.Equal(t, uint64(0), c.DroppedSpans(), "span without active tick should be silently ignored")
}

func TestCollector_SpansCopied(t *testing.T) {
	c := NewCollector(1)
	ch := c.Subscribe()

	now := time.Now()
	c.StartTick()
	c.RecordSpan(TickSpan{SystemName: "a"})
	c.RecordTick(0, now)

	batch := <-ch
	require.Len(t, batch.Ticks, 1)
	require.Len(t, batch.Ticks[0].Spans, 1)

	c.StartTick()
	c.RecordSpan(TickSpan{SystemName: "b"})
	c.RecordTick(1, now)

	assert.Equal(t, "a", batch.Ticks[0].Spans[0].SystemName)
}

// Fix #3: Verify thread safety of Subscribe/Unsubscribe concurrent with the tick loop.
func TestCollector_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	c := NewCollector(1)

	const writerTicks = 200
	const subGoroutines = 8

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		now := time.Now()
		for i := range writerTicks {
			c.StartTick()
			c.RecordSpan(TickSpan{SystemName: "sys", TickHeight: uint64(i)})
			c.RecordTick(uint64(i), now)
			now = now.Add(50 * time.Millisecond)
		}
	}()

	for range subGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range writerTicks / 4 {
				ch := c.Subscribe()
				// Drain a few batches to exercise the send path.
				for range 2 {
					select {
					case <-ch:
					default:
					}
				}
				c.Unsubscribe(ch)
			}
		}()
	}

	wg.Wait()
}

// Fix #10: Double-Unsubscribe should be a harmless no-op.
func TestCollector_DoubleUnsubscribe(t *testing.T) {
	c := NewCollector(1)
	ch := c.Subscribe()
	c.Unsubscribe(ch)
	c.Unsubscribe(ch) // must not panic

	now := time.Now()
	c.StartTick()
	c.RecordTick(0, now)

	select {
	case <-ch:
		t.Fatal("should not receive after double unsubscribe")
	default:
	}
}

// Fix #10: Reset while subscribers exist should not emit partial data.
func TestCollector_ResetWithSubscribers(t *testing.T) {
	c := NewCollector(5)
	ch := c.Subscribe()

	now := time.Now()
	for i := range 3 {
		c.StartTick()
		c.RecordSpan(TickSpan{SystemName: "sys"})
		c.RecordTick(uint64(i), now)
	}

	c.Reset()

	// Continue ticking to a full batch after reset.
	for i := range 5 {
		c.StartTick()
		c.RecordSpan(TickSpan{SystemName: "post-reset"})
		c.RecordTick(uint64(100+i), now)
	}

	batch := <-ch
	assert.Len(t, batch.Ticks, 5, "should receive a full batch from post-reset ticks only")
	assert.Equal(t, uint64(100), batch.Ticks[0].TickHeight, "first tick should be post-reset")
}
