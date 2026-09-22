package cardinal

import (
	"context"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type tracedSystem struct {
	BaseSystemState
	runs int
}

func (s *tracedSystem) Run() { s.runs++ }

// TestTickEmitsSpans checks the span tree a tracing backend receives for one tick: a root tick span
// with one child per system, the event dispatch, and the snapshot write.
func TestTickEmitsSpans(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	off := false
	w, err := NewWorld(WorldOptions{
		Region:              "trace",
		Organization:        "trace",
		Project:             "trace",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1,
		Debug:               &off,
		Pprof:               &off,
	})
	require.NoError(t, err)

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	w.tel.Tracer = provider.Tracer("test")

	sys := &tracedSystem{}
	w.RegisterSystemV2(sys, WithHook(PostUpdate))
	w.init()
	exporter.Reset()

	w.Tick(time.Now())
	require.Equal(t, 1, sys.runs)

	spans := exporter.GetSpans()
	byName := map[string]tracetest.SpanStub{}
	for _, s := range spans {
		byName[s.Name] = s
	}
	require.Len(t, spans, 4, "tick, system, event dispatch, persist state")

	tick := byName[spanTick]
	require.False(t, tick.Parent.IsValid(), "tick is a root span")
	require.Contains(t, tick.Attributes, attrTickHeight.Int64(0))
	require.Contains(t, tick.Attributes, attrTickCommands.Int(0))

	for _, name := range []string{spanSystem, spanEventDispatch, spanPersistState} {
		child, ok := byName[name]
		require.True(t, ok, "missing span %s", name)
		require.Equal(t, tick.SpanContext.SpanID(), child.Parent.SpanID(), "%s is a child of the tick", name)
	}
	require.Contains(t, byName[spanSystem].Attributes, attrSystemName.String("*cardinal.tracedSystem"))
	require.Contains(t, byName[spanSystem].Attributes, attrSystemHook.String("SYSTEM_HOOK_POST_UPDATE"))
	require.Contains(t, byName[spanPersistState].Attributes, attrSnapshotDue.Bool(true))
}
