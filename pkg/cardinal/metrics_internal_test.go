package cardinal

import (
	"context"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newRecordingMeter installs a meter provider read by hand as the global provider, so a world built
// after it records into the returned reader. Tests that call it must not run in parallel.
func newRecordingMeter(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return reader
}

// collectedMetrics is one collection, flattened for assertions: counter values by metric name and
// the data point's attributes, and histogram counts by metric name.
type collectedMetrics struct {
	sums       map[string]map[attribute.Distinct]int64
	histograms map[string]uint64
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) collectedMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	got := collectedMetrics{sums: map[string]map[attribute.Distinct]int64{}, histograms: map[string]uint64{}}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				got.sums[m.Name] = map[attribute.Distinct]int64{}
				for _, dp := range data.DataPoints {
					got.sums[m.Name][dp.Attributes.Equivalent()] = dp.Value
				}
			case metricdata.Histogram[float64]:
				for _, dp := range data.DataPoints {
					got.histograms[m.Name] += dp.Count
				}
			}
		}
	}
	return got
}

func attrs(kvs ...attribute.KeyValue) attribute.Distinct {
	set := attribute.NewSet(kvs...)
	return set.Equivalent()
}

// slowFirstTickSystem broadcasts one event per tick and holds its first tick past the tick interval.
type slowFirstTickSystem struct {
	delay time.Duration
	runs  int
}

func (s *slowFirstTickSystem) Run(w *World) {
	if s.runs == 0 {
		time.Sleep(s.delay)
	}
	s.runs++
	w.Broadcast(testutils.SimpleEvent{Value: s.runs})
}

// TestTickRecordsMetrics runs two ticks at 10 Hz, the first held past its 100ms budget, with one
// command and one event per tick, and checks every tick metric against those counts.
func TestTickRecordsMetrics(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	reader := newRecordingMeter(t)

	off := false
	w, err := NewWorld(WorldOptions{
		Region:              "metrics",
		Organization:        "metrics",
		Project:             "metrics",
		ShardID:             "0",
		TickRate:            10,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1000,
		Debug:               &off,
	})
	require.NoError(t, err)
	w.RegisterCommand[testutils.SimpleCommand]()
	w.RegisterEvent[testutils.SimpleEvent]()
	w.RegisterSystem(&slowFirstTickSystem{delay: 150 * time.Millisecond})
	w.init()

	enqueueCommand := func() {
		require.NoError(t, w.commands.Enqueue(context.Background(), &iscv1.Command{
			Name:    testutils.SimpleCommand{}.Name(),
			Address: w.address,
			Payload: testutils.SimpleCommand{Value: 7}.MarshalWire(),
		}, command.PlayerSender("player-1")))
	}
	enqueueCommand()
	w.Tick(time.Now())
	enqueueCommand()
	w.Tick(time.Now())

	got := collect(t, reader)
	require.Equal(t, uint64(2), got.histograms[metricTickDuration])
	require.Equal(t, map[attribute.Distinct]int64{attrs(): 1}, got.sums[metricTickOverruns])
	require.Equal(t, map[attribute.Distinct]int64{attrs(attrCommandName.String("simple_command")): 2},
		got.sums[metricTickCommands])
	require.Equal(t, map[attribute.Distinct]int64{attrs(attrEventType.String("testutils.SimpleEvent")): 2},
		got.sums[metricEventsSent])
}

// TestCountingAllocatesNothing checks that counting a command or event, which runs for every one a
// tick handles, allocates nothing once the command is registered.
func TestCountingAllocatesNothing(t *testing.T) {
	newRecordingMeter(t)
	m, err := newWorldMetrics(0)
	require.NoError(t, err)
	m.registerCommand("simple_command")
	event := addOptions(attrEventType.String("testutils.SimpleEvent"))
	ctx := context.Background()

	allocs := testing.AllocsPerRun(100, func() {
		m.countCommand(ctx, "simple_command")
		m.countEvent(ctx, event)
	})
	require.Zero(t, allocs)
}
