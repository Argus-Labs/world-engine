package sentry

import (
	"context"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel/trace"
)

type Options struct {
	Dsn         string
	Environment string
	Tags        map[string]string
}

// New sets up Sentry using the provided options.
// If the DSN is empty, initialization is skipped.
func New(opt Options) error {
	if opt.Dsn == "" {
		// Sentry is disabled if DSN is empty
		return nil
	}

	err := sentrygo.Init(sentrygo.ClientOptions{
		Dsn:         opt.Dsn,
		Environment: opt.Environment,
		Tags:        opt.Tags,
	})
	if err != nil {
		return eris.Wrap(err, "failed to initialize sentry")
	}

	return nil
}

// RecoverAndFlush captures a panic (if any) and flushes buffered events.
// If repanic is true, the panic is rethrown after flush to preserve crash semantics.
func RecoverAndFlush(repanic bool) {
	if !isInitialized() {
		return
	}
	if r := recover(); r != nil {
		sentrygo.CurrentHub().Recover(r)
		sentrygo.Flush(5 * time.Second)
		if repanic {
			panic(r)
		}
		return
	}
	sentrygo.Flush(5 * time.Second)
}

// CaptureException reports a handled error to Sentry.
func CaptureException(ctx context.Context, err error) {
	if !isInitialized() || err == nil {
		return
	}
	sentrygo.WithScope(func(scope *sentrygo.Scope) {
		// Extract OTel trace context
		if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
			scope.SetTag("trace_id", spanCtx.TraceID().String())
			scope.SetTag("span_id", spanCtx.SpanID().String())
		}
		sentrygo.CaptureException(err)
	})
}

// Shutdown flushes buffered events with the provided timeout or context deadline.
func Shutdown(ctx context.Context, timeout time.Duration) {
	if !isInitialized() {
		return
	}
	t := timeout
	if dl, ok := ctx.Deadline(); ok {
		if until := time.Until(dl); until > 0 && until < t {
			t = until
		}
	}
	if t <= 0 {
		t = 1 * time.Second
	}
	sentrygo.Flush(t)
}

// isInitialized checks if Sentry is initialized.
func isInitialized() bool {
	return sentrygo.CurrentHub().Client() != nil
}
