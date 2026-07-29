package performance

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_FlushesAtBatchSize(t *testing.T) {
	c := NewCollector(3)
	ch := c.SubscribeProfiles()

	now := time.Now()
	for i := range 2 {
		captureSystemSpans := c.StartTick()
		c.RecordSpan(TickSpan{SystemName: "sys"})
		c.RecordTick(captureSystemSpans, uint64(i), now, time.Now().Add(-5*time.Millisecond))
		now = now.Add(50 * time.Millisecond)
	}

	select {
	case <-ch:
		t.Fatal("should not receive a batch before reaching batch size")
	default:
	}

	captureSystemSpans := c.StartTick()
	c.RecordSpan(TickSpan{SystemName: "sys"})
	c.RecordTick(captureSystemSpans, 2, now, time.Now().Add(-5*time.Millisecond))

	select {
	case batch := <-ch:
		assert.Len(t, batch.Ticks, 3)
		assert.Equal(t, uint64(0), batch.Ticks[0].TickHeight)
		assert.Equal(t, uint64(2), batch.Ticks[2].TickHeight)
		assert.GreaterOrEqual(t, batch.Ticks[0].SystemPhaseElapsed, 5*time.Millisecond)
		assert.Len(t, batch.Ticks[0].Spans, 1)
	default:
		t.Fatal("expected a batch after reaching batch size")
	}
}

func TestCollector_MultipleSubscribers(t *testing.T) {
	c := NewCollector(1)
	ch1 := c.SubscribeTimings()
	ch2 := c.SubscribeTimings()

	now := time.Now()
	captureSystemSpans := c.StartTick()
	c.RecordTick(captureSystemSpans, 0, now, time.Now())

	batch1 := <-ch1
	batch2 := <-ch2
	assert.Len(t, batch1.Ticks, 1)
	assert.Len(t, batch2.Ticks, 1)
}

func TestCollector_UnsubscribeIsIdempotent(t *testing.T) {
	c := NewCollector(1)
	ch := c.SubscribeTimings()
	c.Unsubscribe(ch)
	c.Unsubscribe(ch)

	now := time.Now()
	captureSystemSpans := c.StartTick()
	c.RecordTick(captureSystemSpans, 0, now, time.Now())

	select {
	case <-ch:
		t.Fatal("should not receive a batch after unsubscribe")
	default:
	}
}

func TestCollector_NonBlockingSend(t *testing.T) {
	c := NewCollector(1)
	c.SubscribeTimings()

	done := make(chan struct{})
	go func() {
		defer close(done)
		now := time.Now()
		for i := range 100 {
			captureSystemSpans := c.StartTick()
			c.RecordTick(captureSystemSpans, uint64(i), now, time.Now())
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("recording blocked on a slow subscriber")
	}
}

func TestCollector_SpansCopied(t *testing.T) {
	c := NewCollector(1)
	ch := c.SubscribeProfiles()

	now := time.Now()
	captureSystemSpans := c.StartTick()
	c.RecordSpan(TickSpan{SystemName: "a"})
	c.RecordTick(captureSystemSpans, 0, now, time.Now())

	batch := <-ch
	require.Len(t, batch.Ticks, 1)
	require.Len(t, batch.Ticks[0].Spans, 1)

	captureSystemSpans = c.StartTick()
	c.RecordSpan(TickSpan{SystemName: "b"})
	c.RecordTick(captureSystemSpans, 1, now, time.Now())

	assert.Equal(t, "a", batch.Ticks[0].Spans[0].SystemName)
}

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
			captureSystemSpans := c.StartTick()
			if captureSystemSpans {
				c.RecordSpan(TickSpan{SystemName: "sys"})
			}
			c.RecordTick(captureSystemSpans, uint64(i), now, time.Now())
			now = now.Add(50 * time.Millisecond)
		}
	}()

	for range subGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range writerTicks / 4 {
				ch := c.SubscribeProfiles()
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

func TestCollector_ResetClearsPendingTicks(t *testing.T) {
	c := NewCollector(5)
	ch := c.SubscribeProfiles()

	now := time.Now()
	for i := range 3 {
		captureSystemSpans := c.StartTick()
		c.RecordSpan(TickSpan{SystemName: "sys"})
		c.RecordTick(captureSystemSpans, uint64(i), now, time.Now())
	}

	c.Reset()

	// Continue ticking to a full batch after reset.
	for i := range 5 {
		captureSystemSpans := c.StartTick()
		c.RecordSpan(TickSpan{SystemName: "post-reset"})
		c.RecordTick(captureSystemSpans, uint64(100+i), now, time.Now())
	}

	batch := <-ch
	assert.Len(t, batch.Ticks, 5, "should receive a full batch from post-reset ticks only")
	assert.Equal(t, uint64(100), batch.Ticks[0].TickHeight, "first tick should be post-reset")
}

func TestCollector_SystemSpanCaptureRequiresProfileSubscriber(t *testing.T) {
	c := NewCollector(1)

	assert.False(t, c.StartTick(), "no subscriber")

	timingsCh := c.SubscribeTimings()
	captureSystemSpans := c.StartTick()
	assert.False(t, captureSystemSpans, "timing subscriber")
	c.RecordTick(captureSystemSpans, 0, time.Now(), time.Now().Add(-2*time.Millisecond))

	timingsBatch := <-timingsCh
	require.Len(t, timingsBatch.Ticks, 1)
	assert.Empty(t, timingsBatch.Ticks[0].Spans)
	assert.GreaterOrEqual(t, timingsBatch.Ticks[0].SystemPhaseElapsed, 2*time.Millisecond)

	profilesCh := c.SubscribeProfiles()
	captureSystemSpans = c.StartTick()
	assert.True(t, captureSystemSpans, "profile subscriber")
	c.RecordSpan(TickSpan{SystemName: "sys"})
	c.RecordTick(captureSystemSpans, 1, time.Now(), time.Now().Add(-3*time.Millisecond))

	assert.Len(t, (<-profilesCh).Ticks[0].Spans, 1)
	<-timingsCh

	c.Unsubscribe(profilesCh)
	assert.False(t, c.StartTick(), "profile subscriber disconnected")
}
