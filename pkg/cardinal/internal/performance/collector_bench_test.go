package performance_test

import (
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/performance"
)

func BenchmarkCollectorCapture(b *testing.B) {
	const systemCount = 50

	b.Run("debug_idle", func(b *testing.B) {
		c := performance.NewCollector(10)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			captureSystemSpans := c.StartTick()
			start := time.Now()
			c.RecordTick(captureSystemSpans, uint64(i), start, time.Since(start))
		}
	})

	b.Run("watch_timing", func(b *testing.B) {
		c := performance.NewCollector(10)
		ch := c.SubscribeTimings()
		defer c.Unsubscribe(ch)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			captureSystemSpans := c.StartTick()
			start := time.Now()
			c.RecordTick(captureSystemSpans, uint64(i), start, time.Since(start))
		}
	})

	b.Run("profile_50_systems", func(b *testing.B) {
		c := performance.NewCollector(10)
		ch := c.SubscribeProfiles()
		defer c.Unsubscribe(ch)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			captureSystemSpans := c.StartTick()
			tickStart := time.Now()
			for systemIndex := range systemCount {
				systemStart := time.Now()
				c.RecordSpan(performance.TickSpan{
					SystemName: "system",
					SystemHook: uint8(systemIndex % 3),
					StartTime:  systemStart,
					EndTime:    time.Now(),
				})
			}
			c.RecordTick(captureSystemSpans, uint64(i), tickStart, time.Since(tickStart))
		}
	})
}
