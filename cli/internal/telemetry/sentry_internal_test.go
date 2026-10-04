package telemetry

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the panic-ownership contract between SentryFlush and the
// top-level exit-code handler in cmd/world/main.go. Go unwinds defers LIFO and
// the first recover() during unwinding "owns" the panic; a later recover()
// returns nil. main() defers its exit-code handler BEFORE SentryFlush, so
// SentryFlush runs first during unwinding. Before the fix, SentryFlush's
// recover() owned the panic and returned normally, so main's handler saw
// recover()==nil and ran os.Exit(0) — laundering an unrecovered crash into a
// successful exit. After the fix, SentryFlush re-raises the panic after
// capturing and flushing the Sentry event, so the panic keeps unwinding and
// main's handler sees it and re-enters the crash path (exit code 2 + trace).

// flushRequiredTransport models the real HTTPTransport: SendEvent enqueues to
// an internal buffer and only Flush "delivers" events. It distinguishes "event
// was queued" (pending) from "event was flushed before re-panic" (delivered),
// guarding the ordering invariant the bug report calls out — Recover() and
// Flush() must run BEFORE the re-panic, otherwise re-panicking would terminate
// the process before the event reaches the network.
type flushRequiredTransport struct {
	mu        sync.Mutex
	pending   []*sentry.Event
	delivered []*sentry.Event
}

func (t *flushRequiredTransport) Configure(sentry.ClientOptions) {}

func (t *flushRequiredTransport) SendEvent(event *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending = append(t.pending, event)
}

func (t *flushRequiredTransport) Flush(time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.delivered = append(t.delivered, t.pending...)
	t.pending = nil
	return true
}

func (t *flushRequiredTransport) FlushWithContext(context.Context) bool {
	return t.Flush(0)
}

func (t *flushRequiredTransport) Close() {
	t.Flush(0)
}

func (t *flushRequiredTransport) Delivered() []*sentry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	copied := make([]*sentry.Event, len(t.delivered))
	copy(copied, t.delivered)
	return copied
}

// bindTestSentry initializes the real sentry SDK on the global hub with a
// transport that performs no network I/O, returning a cleanup that unbinds the
// client so later tests are isolated. Integrations are disabled to keep the
// event pipeline deterministic (no source-context file reads) and to guarantee
// no integration drops the event.
func bindTestSentry(t *testing.T, transport sentry.Transport) func() {
	t.Helper()
	err := sentry.Init(sentry.ClientOptions{
		// A syntactically valid DSN is required for NewClient to succeed; no HTTP
		// traffic occurs because the transport is injected.
		Dsn:              "http://fakekey@127.0.0.1:8123/1",
		SampleRate:       1.0,
		AttachStacktrace: true,
		Integrations:     func([]sentry.Integration) []sentry.Integration { return nil },
		Transport:        transport,
	})
	require.NoError(t, err)
	return func() {
		// Unbind so no later test observes this client or its buffered events.
		sentry.CurrentHub().BindClient(nil)
	}
}

// withPanicUnwind simulates main()'s defer ordering: outer is deferred BEFORE
// inner, so during unwinding inner runs first (as main's `defer SentryFlush()`
// does), then outer (as main's top-level handler does). The body panic stands
// in for a panic raised during ctx.Run().
func withPanicUnwind(outer, inner func(), value any) {
	defer outer()
	defer inner()
	panic(value)
}

func TestSentryFlush_NoPanicInFlight_FlushesAndResetsWithoutRePanicking(t *testing.T) {
	// No client is bound on purpose: hub.Recover/hub.Flush are no-ops, so this
	// isolates SentryFlush's control flow on the happy path (reset, no re-panic).
	prev := sentryInitialized
	sentryInitialized = true
	defer func() { sentryInitialized = prev }()

	assert.NotPanics(t, func() { SentryFlush() })

	assert.False(t, sentryInitialized, "sentryInitialized must be reset to false after flush")
}

func TestSentryFlush_ReRaisesPanicSoOuterHandlerSeesIt(t *testing.T) {
	// The core regression test: before the fix, SentryFlush's recover() consumed
	// the panic and did not re-raise, so the outer handler (main's exit-code
	// handler) saw recover()==nil and ran os.Exit(0), laundering a crash into a
	// successful exit. After the fix, the outer handler MUST observe the panic.
	prev := sentryInitialized
	sentryInitialized = true
	defer func() { sentryInitialized = prev }()

	var caught any
	withPanicUnwind(
		func() { caught = recover() }, // outer defer: runs LAST (mimics main handler)
		SentryFlush,                   // inner defer: runs FIRST (mimics deferred SentryFlush)
		"boom from ctx.Run()",         // the simulated crash
	)

	require.NotNil(t, caught, "outer handler must see the re-raised panic (the fix)")
	assert.Equal(t, "boom from ctx.Run()", caught)
	assert.False(t, sentryInitialized, "flag must be reset even on the re-panic path")
}

func TestSentryFlush_CapturesAndFlushesBeforeRaisingSoEventIsDelivered(t *testing.T) {
	// Guards the ordering the bug report warns about: moving CurrentHub().Recover
	// out of SentryFlush (e.g. into main's handler) and reducing SentryFlush to a
	// bare Flush would cause SentryFlush to flush BEFORE the event is queued, and
	// the re-panic would terminate the process before any later flush — silently
	// dropping the Sentry event. The flushRequiredTransport only records events
	// as "delivered" on Flush, so any regression that re-panics before flushing
	// leaves Delivered() empty and fails this test.
	transport := &flushRequiredTransport{}
	cleanup := bindTestSentry(t, transport)
	defer cleanup()

	prev := sentryInitialized
	sentryInitialized = true
	defer func() { sentryInitialized = prev }()

	var caught any
	withPanicUnwind(
		func() { caught = recover() },
		SentryFlush,
		"boom from ctx.Run()",
	)

	require.NotNil(t, caught)
	delivered := transport.Delivered()
	require.Len(t, delivered, 1, "exactly one fatal event must be delivered before the re-panic")
	assert.Equal(t, sentry.LevelFatal, delivered[0].Level, "Recover must tag the event as fatal")
	assert.Equal(t, "boom from ctx.Run()", delivered[0].Message)
}

func TestSentryFlush_WhenNotInitialized_DoesNotConsumePanic(t *testing.T) {
	// The local-dev / empty-DSN build where sentryInitialized is false: SentryFlush
	// must be a no-op and must NOT recover() the panic, so it propagates intact to
	// the outer handler. This is the (already correct, unchanged) Sentry-OFF path
	// the bug report documents as exiting 2 with a full stack trace.
	prev := sentryInitialized
	sentryInitialized = false
	defer func() { sentryInitialized = prev }()

	var caught any
	withPanicUnwind(
		func() { caught = recover() },
		SentryFlush,
		"unrecovered",
	)

	assert.Equal(t, "unrecovered", caught, "panic must propagate untouched when sentry is off")
	assert.False(t, sentryInitialized)
}
