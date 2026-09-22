package cardinal

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Span names and attribute keys emitted by the tick loop. Keep these stable: dashboards and
// alerts key on them.
const (
	spanInit           = "cardinal.init"
	spanRestore        = "cardinal.restore"
	spanTick           = "cardinal.tick"
	spanSystem         = "cardinal.system"
	spanEventDispatch  = "cardinal.events.dispatch"
	spanPersistState   = "cardinal.persist_state"
	spanEventPublish   = "cardinal.event.publish"
	spanInterShardSend = "cardinal.command.send"

	attrTickHeight   = attribute.Key("cardinal.tick.height")
	attrTickCommands = attribute.Key("cardinal.tick.commands")
	attrSystemName   = attribute.Key("cardinal.system.name")
	attrSystemHook   = attribute.Key("cardinal.system.hook")
	attrSnapshotDue  = attribute.Key("cardinal.snapshot.due")

	attrCommandName      = attribute.Key("cardinal.command.name")
	attrCommandPersona   = attribute.Key("cardinal.command.persona")
	attrCommandTarget    = attribute.Key("cardinal.command.target")
	attrEventName        = attribute.Key("cardinal.event.name")
	attrEventRecipient   = attribute.Key("cardinal.event.recipient")
	attrEventSubscribers = attribute.Key("cardinal.event.subscribers")
	attrEventWaiters     = attribute.Key("cardinal.event.waiters")
)

// startSpan starts a child span of ctx. Worlds built as struct literals (tests) have a nil tracer
// and nil tickCtx and get a no-op span, matching the nil-safe debug module.
func (w *World) startSpan(
	ctx context.Context, name string, attrs ...attribute.KeyValue,
) (context.Context, trace.Span) {
	tracer := w.tel.Tracer
	if tracer == nil {
		tracer = noop.Tracer{}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Callers own the returned span and end it themselves.
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...)) //nolint:spancheck // ended by caller
}

// endSpan ends span and marks it failed when err is non-nil.
func endSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
