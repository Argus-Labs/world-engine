package performance

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorPublishesCompletedTicks(t *testing.T) {
	c := NewCollector(4)
	first := c.SubscribeTimings()
	second := c.SubscribeTimings()

	now := time.Now()
	captureSystemSpans := c.StartTick()
	c.RecordTick(captureSystemSpans, 42, now, now.Add(-5*time.Millisecond))

	for _, ch := range []<-chan TickTimeline{first, second} {
		tick := <-ch
		assert.Equal(t, uint64(42), tick.TickHeight)
		assert.GreaterOrEqual(t, tick.SystemPhaseElapsed, 5*time.Millisecond)
	}
}

func TestCollectorStartsSubscriptionsAtTickBoundary(t *testing.T) {
	c := NewCollector(4)
	captureSystemSpans := c.StartTick()

	ch := c.SubscribeTimings()
	c.RecordTick(captureSystemSpans, 0, time.Now(), time.Now())
	select {
	case <-ch:
		t.Fatal("a new subscriber must not receive a tick already in progress")
	default:
	}

	captureSystemSpans = c.StartTick()
	c.RecordTick(captureSystemSpans, 1, time.Now(), time.Now())
	assert.Equal(t, uint64(1), (<-ch).TickHeight)
}

func TestCollectorKeepsLatestTicksForSlowSubscriber(t *testing.T) {
	c := NewCollector(2)
	ch := c.SubscribeTimings()
	c.StartTick()

	for i := range 3 {
		c.RecordTick(TickCapture{}, uint64(i), time.Now(), time.Now())
	}

	assert.Equal(t, uint64(1), (<-ch).TickHeight)
	assert.Equal(t, uint64(2), (<-ch).TickHeight)
}

func TestCollectorUnsubscribeIsIdempotent(t *testing.T) {
	c := NewCollector(1)
	ch := c.SubscribeTimings()
	c.Unsubscribe(ch)
	c.Unsubscribe(ch)

	c.StartTick()
	c.RecordTick(TickCapture{}, 0, time.Now(), time.Now())
	select {
	case <-ch:
		t.Fatal("should not receive a tick after unsubscribe")
	default:
	}
}

func TestCollectorCopiesSpans(t *testing.T) {
	c := NewCollector(2)
	ch := c.SubscribeProfiles()

	captureSystemSpans := c.StartTick()
	c.RecordSpan(TickSpan{SystemName: "a"})
	c.RecordTick(captureSystemSpans, 0, time.Now(), time.Now())
	first := <-ch
	require.Len(t, first.Spans, 1)

	captureSystemSpans = c.StartTick()
	c.RecordSpan(TickSpan{SystemName: "b"})
	c.RecordTick(captureSystemSpans, 1, time.Now(), time.Now())

	assert.Equal(t, "a", first.Spans[0].SystemName)
}

func TestCollectorResetDropsOldTicks(t *testing.T) {
	c := NewCollector(4)
	ch := c.SubscribeTimings()
	c.StartTick()
	c.RecordTick(TickCapture{}, 7, time.Now(), time.Now())

	c.Reset()
	select {
	case <-ch:
		t.Fatal("reset should clear queued ticks")
	default:
	}

	capture := c.StartTick()
	c.RecordTick(capture, 0, time.Now(), time.Now())
	tick := <-ch
	assert.Equal(t, uint64(0), tick.TickHeight)
	assert.Equal(t, uint64(1), tick.Generation)
}

func TestCollectorDropsTickStartedBeforeReset(t *testing.T) {
	c := NewCollector(4)
	ch := c.SubscribeTimings()
	oldCapture := c.StartTick()

	c.Reset()
	c.RecordTick(oldCapture, 7, time.Now(), time.Now())
	select {
	case <-ch:
		t.Fatal("a tick started before reset must not appear after reset")
	default:
	}

	newCapture := c.StartTick()
	c.RecordTick(newCapture, 0, time.Now(), time.Now())
	assert.Equal(t, uint64(0), (<-ch).TickHeight)
}

func TestCollectorProfilesOnlyCompleteTicks(t *testing.T) {
	c := NewCollector(4)
	timings := c.SubscribeTimings()

	captureSystemSpans := c.StartTick()
	require.False(t, captureSystemSpans.SystemSpans)
	profiles := c.SubscribeProfiles()
	c.RecordTick(captureSystemSpans, 0, time.Now(), time.Now())
	assert.Equal(t, uint64(0), (<-timings).TickHeight)
	select {
	case <-profiles:
		t.Fatal("a profile subscriber must not receive a partially captured tick")
	default:
	}

	captureSystemSpans = c.StartTick()
	require.True(t, captureSystemSpans.SystemSpans)
	c.RecordSpan(TickSpan{SystemName: "system"})
	c.RecordTick(captureSystemSpans, 1, time.Now(), time.Now())
	profile := <-profiles
	assert.Equal(t, uint64(1), profile.TickHeight)
	assert.Len(t, profile.Spans, 1)
}

func TestCollectorFinishesLatchedCaptureAfterUnsubscribe(t *testing.T) {
	c := NewCollector(4)
	timings := c.SubscribeTimings()
	profiles := c.SubscribeProfiles()

	captureSystemSpans := c.StartTick()
	require.True(t, captureSystemSpans.SystemSpans)
	c.RecordSpan(TickSpan{SystemName: "system"})
	c.Unsubscribe(profiles)
	c.RecordTick(captureSystemSpans, 1, time.Now(), time.Now())

	tick := <-timings
	assert.Len(t, tick.Spans, 1)
	assert.False(t, c.StartTick().SystemSpans, "the next tick should not capture spans")
}

func TestCollectorConcurrentSubscribeUnsubscribe(t *testing.T) {
	c := NewCollector(4)

	const writerTicks = 200
	const subGoroutines = 8
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range writerTicks {
			captureSystemSpans := c.StartTick()
			if captureSystemSpans.SystemSpans {
				c.RecordSpan(TickSpan{SystemName: "system"})
			}
			c.RecordTick(captureSystemSpans, uint64(i), time.Now(), time.Now())
		}
	}()

	for range subGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range writerTicks / 4 {
				ch := c.SubscribeProfiles()
				select {
				case <-ch:
				default:
				}
				c.Unsubscribe(ch)
			}
		}()
	}

	wg.Wait()
}
