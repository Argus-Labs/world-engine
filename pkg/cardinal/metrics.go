package cardinal

import (
	"context"
	"errors"
	"time"

	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// meterName is the instrumentation scope of Cardinal's metrics.
const meterName = "github.com/argus-labs/world-engine/pkg/cardinal"

// Metric names and attribute keys emitted by the tick loop. Keep these stable: dashboards and
// alerts key on them. They are exported only when OTEL_EXPORTER_OTLP_METRICS_ENDPOINT is set.
const (
	metricTickDuration = "cardinal.tick.duration"
	metricTickOverruns = "cardinal.tick.overruns"
	metricTickCommands = "cardinal.tick.commands"
	metricEventsSent   = "cardinal.events.sent"

	// attrEventType is the event's Go type, not its Name(): some events derive their name from
	// instance data, which would give the metric one series per request.
	attrEventType = attribute.Key("cardinal.event.type")
)

// tickDurationBuckets resolve the common tick budgets (60, 30 and 20 Hz), so the histogram shows
// how close ticks run to them. In seconds, the OTel unit for durations.
var tickDurationBuckets = []float64{ //nolint:gochecknoglobals // constant bucket layout
	0.001, 0.0025, 0.005, 0.01, 0.0167, 0.025, 0.0333, 0.05, 0.075, 0.1, 0.25, 0.5, 1,
}

// worldMetrics are the instruments the tick loop records into. They come from the global meter
// provider when the world is built, so a provider installed by telemetry.New, or by a test before
// NewWorld, receives them. Without one they are no-ops. A nil *worldMetrics records nothing, so a
// World built by hand in a test needs none.
type worldMetrics struct {
	tickDuration metric.Float64Histogram
	tickOverruns metric.Int64Counter
	tickCommands metric.Int64Counter
	eventsSent   metric.Int64Counter

	// tickInterval is the tick budget: a longer tick is an overrun. Zero disables overrun counting.
	tickInterval time.Duration

	// commandAttrs holds each registered command's attributes, built once by RegisterCommand as the
	// whole options slice, so counting a command allocates nothing.
	commandAttrs map[string][]metric.AddOption
}

func newWorldMetrics(tickInterval time.Duration) (*worldMetrics, error) {
	meter := otel.Meter(meterName)
	m := &worldMetrics{tickInterval: tickInterval, commandAttrs: make(map[string][]metric.AddOption)}

	var errs, err error
	m.tickDuration, err = meter.Float64Histogram(metricTickDuration, metric.WithUnit("s"),
		metric.WithDescription("Wall time of each tick: draining commands, running systems, dispatching events "+
			"and persisting state."),
		metric.WithExplicitBucketBoundaries(tickDurationBuckets...))
	errs = errors.Join(errs, err)
	m.tickOverruns, err = meter.Int64Counter(metricTickOverruns, metric.WithUnit("{tick}"),
		metric.WithDescription("Ticks that ran longer than the tick interval, putting the shard behind its tick rate."))
	errs = errors.Join(errs, err)
	m.tickCommands, err = meter.Int64Counter(metricTickCommands, metric.WithUnit("{command}"),
		metric.WithDescription("Commands handed to systems, by command name."))
	errs = errors.Join(errs, err)
	m.eventsSent, err = meter.Int64Counter(metricEventsSent, metric.WithUnit("{event}"),
		metric.WithDescription("Events sent to clients with Broadcast or SendTo, by event type."))
	errs = errors.Join(errs, err)

	return m, eris.Wrap(errs, "failed to create cardinal metrics")
}

// registerCommand builds the attributes a command of this name is counted under.
func (m *worldMetrics) registerCommand(name string) {
	if m == nil {
		return
	}
	m.commandAttrs[name] = addOptions(attrCommandName.String(name))
}

// countCommand counts one command handed to systems. Only registered commands reach a tick.
func (m *worldMetrics) countCommand(ctx context.Context, name string) {
	if m == nil {
		return
	}
	if attrs, ok := m.commandAttrs[name]; ok {
		m.tickCommands.Add(ctx, 1, attrs...)
	}
}

// recordTick records one tick's wall time, and an overrun when it exceeded the tick interval.
func (m *worldMetrics) recordTick(ctx context.Context, elapsed time.Duration) {
	if m == nil {
		return
	}
	m.tickDuration.Record(ctx, elapsed.Seconds())
	if m.tickInterval > 0 && elapsed > m.tickInterval {
		m.tickOverruns.Add(ctx, 1)
	}
}

// countEvent counts one event sent to clients, under the attributes RegisterEvent built for its type.
func (m *worldMetrics) countEvent(ctx context.Context, attrs []metric.AddOption) {
	if m == nil {
		return
	}
	m.eventsSent.Add(ctx, 1, attrs...)
}

// addOptions builds the options for counting under attrs once, as a slice: passing one option
// variadically would allocate a fresh slice on every call.
func addOptions(attrs ...attribute.KeyValue) []metric.AddOption {
	return []metric.AddOption{metric.WithAttributeSet(attribute.NewSet(attrs...))}
}
