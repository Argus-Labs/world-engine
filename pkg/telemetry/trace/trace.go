// Package trace is a light wrapper over the OpenTelemetry tracing API, modeled on Sourcegraph's
// internal/trace: one constructor bound to the global tracer provider and a Trace type that adds
// error helpers to the span.
package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// tracerName is the instrumentation scope reported on every span started by New.
const tracerName = "github.com/argus-labs/world-engine"

// Trace is a light wrapper of oteltrace.Span. Use New to construct one.
type Trace struct {
	oteltrace.Span // never nil
}

var _ oteltrace.Span = Trace{}

// New starts a span under the global tracer provider and returns it with the derived context.
// Spans are children of the span in ctx, or roots when ctx carries none. Before telemetry.New
// runs, or when tracing is disabled, the global provider is a no-op, so callers never need a nil
// check. A nil ctx is treated as context.Background().
func New(ctx context.Context, name string, opts ...oteltrace.SpanStartOption) (context.Context, Trace) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Callers own the returned span and end it with End or EndWithErr.
	ctx, span := otel.Tracer(tracerName).Start(ctx, name, opts...) //nolint:spancheck // ended by caller
	return ctx, Trace{span}                                        //nolint:spancheck // ended by caller
}

// SetError declares that this span resulted in an error. A nil err is a no-op.
func (t Trace) SetError(err error) {
	if err == nil {
		return
	}
	t.RecordError(err)
	t.SetStatus(codes.Error, err.Error())
}

// EndWithErr finishes the span and sets its error value. To run it deferred against a named
// return, wrap it in a closure so the final value of err is read:
//
//	ctx, span := trace.New(ctx, "op")
//	defer func() { span.EndWithErr(err) }()
//
// Prefer a direct "defer span.End()" plus SetError where the control flow allows: the SDK's End
// records a panic as an exception event only when it is the deferred call itself.
func (t Trace) EndWithErr(err error) {
	t.SetError(err)
	t.End()
}
