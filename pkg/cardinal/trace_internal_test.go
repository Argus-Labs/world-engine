package cardinal

import (
	"context"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newRecordingTracer installs an in-memory span exporter as the global tracer provider for the
// test and returns it. Tests that call it must not run in parallel.
func newRecordingTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return exporter
}

func spansByName(exporter *tracetest.InMemoryExporter) map[string]tracetest.SpanStub {
	byName := map[string]tracetest.SpanStub{}
	for _, s := range exporter.GetSpans() {
		byName[s.Name] = s
	}
	return byName
}

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

	exporter := newRecordingTracer(t)

	sys := &tracedSystem{}
	w.RegisterSystemV2(sys, WithHook(PostUpdate))
	w.init()
	exporter.Reset()

	w.Tick(time.Now())
	require.Equal(t, 1, sys.runs)

	require.Len(t, exporter.GetSpans(), 4, "tick, system, event dispatch, persist state")
	byName := spansByName(exporter)

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

// TestTickLinksCommandsAndTracesEvents checks that a command enqueued under a request span shows up
// as a link on the tick that drains it, and that each published event gets its own span under the
// dispatch span.
func TestTickLinksCommandsAndTracesEvents(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	off := false
	w, err := NewWorld(WorldOptions{
		Region:              "trace",
		Organization:        "trace",
		Project:             "trace",
		ShardID:             "1",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1000,
		Debug:               &off,
		Pprof:               &off,
	})
	require.NoError(t, err)
	exporter := newRecordingTracer(t)

	_, err = w.commands.Register(testutils.SimpleCommand{}.Name(), command.NewQueue[testutils.SimpleCommand]())
	require.NoError(t, err)
	w.init()

	// Enqueue a command the way the ConnectRPC handler does: under the request's span.
	requestCtx, requestSpan := otel.Tracer("test").Start(context.Background(), "request")
	require.NoError(t, w.commands.Enqueue(requestCtx, &iscv1.Command{
		Name:    testutils.SimpleCommand{}.Name(),
		Address: w.address,
		Persona: &iscv1.Persona{Id: "player-1"},
		Payload: testutils.SimpleCommand{Value: 7}.MarshalWire(),
	}))
	requestSpan.End()

	w.events.Enqueue(event.Event{Kind: event.KindDefault, Payload: testutils.SimpleEvent{Value: 1}, Recipient: "player-1"})
	exporter.Reset()

	w.Tick(time.Now())

	byName := spansByName(exporter)
	tick := byName[spanTick]
	require.Contains(t, tick.Attributes, attrTickCommands.Int(1))
	require.Len(t, tick.Links, 1)
	require.Equal(t, requestSpan.SpanContext().SpanID(), tick.Links[0].SpanContext.SpanID())
	require.Contains(t, tick.Links[0].Attributes, attrCommandName.String(testutils.SimpleCommand{}.Name()))
	require.Contains(t, tick.Links[0].Attributes, attrCommandPersona.String("player-1"))

	publish, ok := byName[spanEventPublish]
	require.True(t, ok, "missing event publish span")
	require.Equal(t, byName[spanEventDispatch].SpanContext.SpanID(), publish.Parent.SpanID())
	require.Contains(t, publish.Attributes, attrEventName.String(testutils.SimpleEvent{}.Name()))
	require.Contains(t, publish.Attributes, attrEventRecipient.String("player-1"))
	require.Contains(t, publish.Attributes, attrEventSubscribers.Int(0))
}

// TestInterShardCommandPropagatesTrace sends a command from shard A to shard B over NATS and checks
// that the command B drains carries A's trace, so B's tick links back into A's trace.
func TestInterShardCommandPropagatesTrace(t *testing.T) {
	// telemetry.New installs this in production; service fixtures skip it.
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previousPropagator) })
	prng := testutils.NewRand(t)

	fixtureA := newServiceFixture(t, prng, true)
	fixtureB := newServiceFixture(t, prng, true)
	exporter := newRecordingTracer(t)

	payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
	require.NoError(t, fixtureA.svc.publishInterShardCommand(context.Background(), event.Event{
		Kind: event.KindInterShardCommand,
		Payload: command.Command{
			Name:    payload.Name(),
			Address: fixtureB.world.address,
			Persona: micro.String(fixtureA.world.address),
			Payload: payload,
		},
	}))

	send, ok := spansByName(exporter)[spanInterShardSend]
	require.True(t, ok, "missing inter-shard send span")
	require.Contains(t, send.Attributes, attrCommandTarget.String(micro.String(fixtureB.world.address)))

	fixtureB.world.commands.Drain()
	cmds, err := fixtureB.world.commands.Get(fixtureB.commandID)
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	require.True(t, cmds[0].Span.IsValid(), "command lost its trace context crossing NATS")
	require.Equal(t, send.SpanContext.TraceID(), cmds[0].Span.TraceID())
}

// panickingEvent is an event payload whose wire encoding panics, as the generated wire methods do
// for an unencodable field. schema.Marshal sizes first, so SizeWire is the one that fires.
type panickingEvent struct{ testutils.SimpleEvent }

func (panickingEvent) SizeWire() int            { panic("unencodable payload") }
func (panickingEvent) AppendWire([]byte) []byte { panic("unencodable payload") }

// TestEventPublishSpanRecordsPanic checks that a payload that panics while encoding is recorded as
// an exception on the publish span, the one span that names the event.
func TestEventPublishSpanRecordsPanic(t *testing.T) {
	prng := testutils.NewRand(t)
	fixture := newServiceFixture(t, prng, false)
	exporter := newRecordingTracer(t)

	require.PanicsWithValue(t, "unencodable payload", func() {
		_ = fixture.svc.publishDefaultEvent(context.Background(), event.Event{
			Kind: event.KindDefault, Payload: panickingEvent{}, Recipient: "player-1",
		})
	})

	publish, ok := spansByName(exporter)[spanEventPublish]
	require.True(t, ok, "publish span was not ended")
	require.Contains(t, publish.Attributes, attrEventName.String(testutils.SimpleEvent{}.Name()))
	eventNames := make([]string, 0, len(publish.Events))
	for _, e := range publish.Events {
		eventNames = append(eventNames, e.Name)
	}
	require.Contains(t, eventNames, "exception")
}
