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
	oteltrace "go.opentelemetry.io/otel/trace"
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
	runs int
}

func (s *tracedSystem) Run(*World) { s.runs++ }

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
	})
	require.NoError(t, err)

	exporter := newRecordingTracer(t)

	sys := &tracedSystem{}
	w.RegisterSystem(sys, WithHook(PostUpdate))
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
	})
	require.NoError(t, err)
	exporter := newRecordingTracer(t)

	w.RegisterCommand[testutils.SimpleCommand]()
	w.init()

	// Enqueue a command the way the ConnectRPC handler does: under the request's span.
	requestCtx, requestSpan := otel.Tracer("test").Start(context.Background(), "request")
	require.NoError(t, w.commands.Enqueue(requestCtx, &iscv1.Command{
		Name:    testutils.SimpleCommand{}.Name(),
		Address: w.address,
		Payload: testutils.SimpleCommand{Value: 7}.MarshalWire(),
	}, command.PlayerSender("player-1")))
	requestSpan.End()

	w.events.Enqueue(
		event.Event{Kind: event.KindDefault, Payload: testutils.SimpleEvent{Value: 1}, Recipient: "player-1"},
	)
	exporter.Reset()

	w.Tick(time.Now())

	byName := spansByName(exporter)
	tick := byName[spanTick]
	require.Contains(t, tick.Attributes, attrTickCommands.Int(1))
	require.Len(t, tick.Links, 1)
	require.Equal(t, requestSpan.SpanContext().SpanID(), tick.Links[0].SpanContext.SpanID())
	require.Contains(t, tick.Links[0].Attributes, attrCommandName.String(testutils.SimpleCommand{}.Name()))
	require.Contains(t, tick.Links[0].Attributes, attrCommandSender.String("player-1"))

	publish, ok := byName[spanEventPublish]
	require.True(t, ok, "missing event publish span")
	require.Equal(t, byName[spanEventDispatch].SpanContext.SpanID(), publish.Parent.SpanID())
	require.Contains(t, publish.Attributes, attrEventName.String(testutils.SimpleEvent{}.Name()))
	require.Contains(t, publish.Attributes, attrEventRecipient.String("player-1"))
	require.Contains(t, publish.Attributes, attrEventSubscribers.Int(0))
}

// TestTickSkipsLinksToUnsampledRequests checks that a command enqueued under a request span the
// sampler dropped adds no link to the tick: that request span was never exported, so the link
// would point at nothing. The command itself is still drained and counted.
func TestTickSkipsLinksToUnsampledRequests(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	off := false
	w, err := NewWorld(WorldOptions{
		Region:              "trace",
		Organization:        "trace",
		Project:             "trace",
		ShardID:             "3",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1000,
		Debug:               &off,
	})
	require.NoError(t, err)
	exporter := newRecordingTracer(t)

	w.RegisterCommand[testutils.SimpleCommand]()
	w.init()

	// A request span from a provider that samples nothing: a valid span context with the sampled
	// flag clear, which is what the ratio sampler hands a dropped SendCommand request.
	dropped := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	requestCtx, requestSpan := dropped.Tracer("test").Start(context.Background(), "request")
	require.True(t, requestSpan.SpanContext().IsValid())
	require.False(t, requestSpan.SpanContext().IsSampled())
	require.NoError(t, w.commands.Enqueue(requestCtx, &iscv1.Command{
		Name:    testutils.SimpleCommand{}.Name(),
		Address: w.address,
		Payload: testutils.SimpleCommand{Value: 7}.MarshalWire(),
	}, command.PlayerSender("player-1")))
	requestSpan.End()
	exporter.Reset()

	w.Tick(time.Now())

	tick := spansByName(exporter)[spanTick]
	require.Contains(t, tick.Attributes, attrTickCommands.Int(1))
	require.Empty(t, tick.Links)
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
			Payload: payload,
		},
	}))
	fixtureA.svc.drainInterShardCommands() // what the tick does after dispatch

	cmds := awaitCommands(t, fixtureB)
	var send tracetest.SpanStub
	require.Eventually(t, func() bool {
		var ok bool
		send, ok = spansByName(exporter)[spanInterShardSend]
		return ok
	}, 5*time.Second, 10*time.Millisecond, "missing inter-shard send span")
	require.Contains(t, send.Attributes, attrCommandTarget.String(micro.String(fixtureB.world.address)))

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

// TestTickCapsCommandLinks checks that a tick draining more traced commands than the link cap still
// reports every command in its attributes while keeping the link count at the cap. With the
// default config the cap equals the SDK's default link limit (128), so nothing is dropped.
func TestTickCapsCommandLinks(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	off := false
	w, err := NewWorld(WorldOptions{
		Region:              "trace",
		Organization:        "trace",
		Project:             "trace",
		ShardID:             "2",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1000,
		Debug:               &off,
	})
	require.NoError(t, err)
	require.Equal(t, 128, w.tel.MaxCommandLinks(), "default cap is the SDK default link limit")
	exporter := newRecordingTracer(t)

	w.RegisterCommand[testutils.SimpleCommand]()
	w.init()

	total := 128 + 5
	requestCtx, requestSpan := otel.Tracer("test").Start(context.Background(), "request")
	for i := range total {
		require.NoError(t, w.commands.Enqueue(requestCtx, &iscv1.Command{
			Name:    testutils.SimpleCommand{}.Name(),
			Address: w.address,
			Payload: testutils.SimpleCommand{Value: i}.MarshalWire(),
		}, command.PlayerSender("player-1")))
	}
	requestSpan.End()
	exporter.Reset()

	w.Tick(time.Now())

	tick := spansByName(exporter)[spanTick]
	require.Contains(t, tick.Attributes, attrTickCommands.Int(total))
	require.Len(t, tick.Links, 128)
	require.Zero(t, tick.DroppedLinks, "links past the cap must not be built and then dropped by the SDK")
}

// enqueueDistinctSampledCommands enqueues n commands under n distinct, sampled request spans,
// mirroring the production ConnectRPC handler which traces each inbound command request
// independently (one server span per request). It returns the request spans' SpanContexts in
// enqueue order so tests can assert which commands the tick span links to. The recording tracer
// must already be installed so the spans carry sampled SpanContexts.
func enqueueDistinctSampledCommands(t *testing.T, w *World, n int) []oteltrace.SpanContext {
	t.Helper()
	tr := otel.Tracer("test")
	scs := make([]oteltrace.SpanContext, n)
	for i := range n {
		ctx, span := tr.Start(context.Background(), "request")
		scs[i] = span.SpanContext()
		require.True(t, scs[i].IsSampled(), "request span must be sampled so it produces a link")
		require.NoError(t, w.commands.Enqueue(ctx, &iscv1.Command{
			Name:    testutils.SimpleCommand{}.Name(),
			Address: w.address,
			Payload: testutils.SimpleCommand{Value: i}.MarshalWire(),
		}, command.PlayerSender("player-1")))
		span.End()
	}
	return scs
}

// keptLinkSpanIDs returns the set of SpanIDs the tick span linked to.
func keptLinkSpanIDs(t *testing.T, span tracetest.SpanStub) map[oteltrace.SpanID]struct{} {
	t.Helper()
	ids := make(map[oteltrace.SpanID]struct{}, len(span.Links))
	for _, l := range span.Links {
		ids[l.SpanContext.SpanID()] = struct{}{}
	}
	return ids
}

// TestTickCapMatchesEnvLinkLimitBelowDefault is the regression test for the link-cap bug. With
// OTEL_SPAN_LINK_COUNT_LIMIT set below the former hardcoded cap (128), the cap must track the env
// var so the cap and the provider share one limit. Before the fix, cardinal built 128 links and
// the SDK dropped (128 - limit) of them; after the fix, cardinal builds exactly `limit` links and
// the SDK keeps all of them — nothing is built only to be dropped.
//
// This is the combined regime from the bug report: linkLimit < 128 AND numCommands > 128, the only
// regime where the old hardcoded cap changed the kept set versus a no-cap baseline.
func TestTickCapMatchesEnvLinkLimitBelowDefault(t *testing.T) {
	const linkLimit = 64    // OTEL_SPAN_LINK_COUNT_LIMIT, below the former hardcoded cap of 128.
	const numCommands = 200 // > 128, so the former hardcoded cap would have fired.

	t.Setenv("OTEL_SPAN_LINK_COUNT_LIMIT", "64")
	t.Setenv("LOG_LEVEL", "disabled")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	off := false
	w, err := NewWorld(WorldOptions{
		Region:              "trace",
		Organization:        "trace",
		Project:             "trace",
		ShardID:             "cap-below-default",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1000,
		Debug:               &off,
	})
	require.NoError(t, err)
	require.Equal(t, linkLimit, w.tel.MaxCommandLinks(),
		"cap must track OTEL_SPAN_LINK_COUNT_LIMIT, not the hardcoded 128")

	exporter := newRecordingTracer(t)

	w.RegisterCommand[testutils.SimpleCommand]()
	w.init()
	scs := enqueueDistinctSampledCommands(t, w, numCommands)
	exporter.Reset()

	w.Tick(time.Now())

	tick := spansByName(exporter)[spanTick]
	require.Contains(t, tick.Attributes, attrTickCommands.Int(numCommands))
	require.Len(t, tick.Links, linkLimit)

	// Decisive: with the cap matching the provider limit, the cap builds exactly linkLimit links
	// and the SDK keeps all of them. Before the fix this was 64 (maxCommandLinks - linkLimit) because
	// the cap built 128 links and the SDK dropped 64 of them; it must now be zero.
	require.Zero(t, tick.DroppedLinks,
		"cap matches provider limit; no link is built only to be dropped by the SDK")

	// The cap keeps the first linkLimit sampled commands (prefix). Commands past the cap must not
	// appear as links: this guards against the cap silently regressing to a larger prefix.
	kept := keptLinkSpanIDs(t, tick)
	for i := range linkLimit {
		_, ok := kept[scs[i].SpanID()]
		require.True(t, ok, "command #%d (within cap) should be linked", i)
	}
	for i := linkLimit; i < numCommands; i++ {
		_, ok := kept[scs[i].SpanID()]
		require.False(t, ok, "command #%d (past cap) should not be linked", i)
	}
}

// TestTickCapMatchesEnvLinkLimitAboveDefault checks the cap also tracks an env limit above the
// default. Before the fix, the hardcoded cap of 128 would have kept only 128 of these 200 links;
// after the fix the cap is 256, all 200 sampled commands are linked and none are dropped.
func TestTickCapMatchesEnvLinkLimitAboveDefault(t *testing.T) {
	const linkLimit = 256   // OTEL_SPAN_LINK_COUNT_LIMIT, above the default 128.
	const numCommands = 200 // < linkLimit, so the cap does not fire.

	t.Setenv("OTEL_SPAN_LINK_COUNT_LIMIT", "256")
	t.Setenv("LOG_LEVEL", "disabled")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	off := false
	w, err := NewWorld(WorldOptions{
		Region:              "trace",
		Organization:        "trace",
		Project:             "trace",
		ShardID:             "cap-above-default",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1000,
		Debug:               &off,
	})
	require.NoError(t, err)
	require.Equal(t, linkLimit, w.tel.MaxCommandLinks(),
		"cap must track OTEL_SPAN_LINK_COUNT_LIMIT above the default")

	exporter := newRecordingTracer(t)

	w.RegisterCommand[testutils.SimpleCommand]()
	w.init()
	scs := enqueueDistinctSampledCommands(t, w, numCommands)
	exporter.Reset()

	w.Tick(time.Now())

	tick := spansByName(exporter)[spanTick]
	require.Contains(t, tick.Attributes, attrTickCommands.Int(numCommands))
	require.Len(t, tick.Links, numCommands, "all sampled commands link when below the cap")
	require.Zero(t, tick.DroppedLinks)

	kept := keptLinkSpanIDs(t, tick)
	for i := range numCommands {
		_, ok := kept[scs[i].SpanID()]
		require.True(t, ok, "command #%d should be linked (below cap)", i)
	}
}
