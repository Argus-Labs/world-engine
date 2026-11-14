package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/posthog/posthog-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the two facets of the "Running event dropped on kong's
// early-exit paths" bug:
//
//  1. PosthogInit must NOT stamp the daily dedupe file: the stamp may only be
//     written after a Running event is actually enqueued, so a path that never
//     captures (or drops the flush) does not consume the day's quota.
//  2. FlushTelemetry must drain every sink in the order main's defers run them
//     (PostHog before Sentry), so kong.Exit can flush before os.Exit and the
//     buffered Running event is delivered on --help/--version/parse errors.
//
// The PostHog client is injected as a posthogClientAPI fake so the tests never
// touch the network or the real in-memory queue. HOME is redirected to a temp
// directory so the dedupe file (~/.worldcli/last_logged_time) is isolated.

const exitPanicSentinel = "telemetry-test-exit-sentinel"

// recordingPosthogClient is a posthogClientAPI fake that records Enqueue/Close.
// rec, when non-nil, records the order Close runs relative to other sinks.
type recordingPosthogClient struct {
	mu         sync.Mutex
	enqueued   []posthog.Capture
	closed     int
	enqueueErr error
	closeErr   error
	rec        *orderRecorder
}

func (f *recordingPosthogClient) Enqueue(m posthog.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.enqueueErr != nil {
		return f.enqueueErr
	}
	if c, ok := m.(posthog.Capture); ok {
		f.enqueued = append(f.enqueued, c)
	}
	return nil
}

func (f *recordingPosthogClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	if f.rec != nil {
		f.rec.record("posthog-close")
	}
	return f.closeErr
}

func (f *recordingPosthogClient) Enqueued() []posthog.Capture {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]posthog.Capture, len(f.enqueued))
	copy(out, f.enqueued)
	return out
}

func (f *recordingPosthogClient) Closed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// orderRecorder records an ordered list of sink events. FlushTelemetry runs its
// sinks sequentially on one goroutine, so no locking is strictly needed, but
// the mutex keeps it safe if that ever changes.
type orderRecorder struct {
	mu  sync.Mutex
	log []string
}

func (r *orderRecorder) record(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, s)
}

func (r *orderRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.log))
	copy(out, r.log)
	return out
}

// orderSentryTransport is a sentry.Transport whose Flush records into an
// orderRecorder, so FlushTelemetry's PostHog-before-Sentry order is observable.
type orderSentryTransport struct {
	rec *orderRecorder
}

func (o *orderSentryTransport) Configure(sentry.ClientOptions) {}
func (o *orderSentryTransport) SendEvent(*sentry.Event)        {}
func (o *orderSentryTransport) Flush(time.Duration) bool       { o.rec.record("sentry-flush"); return true }
func (o *orderSentryTransport) FlushWithContext(context.Context) bool {
	return o.Flush(0)
}
func (o *orderSentryTransport) Close() { o.Flush(0) }

// resetPosthogGlobals clears the package-level PostHog state and restores it on
// cleanup so tests are isolated. Call at the top of every PostHog test.
func resetPosthogGlobals(t *testing.T) {
	t.Helper()
	prevClient := posthogClient
	prevInit := posthogInitialized
	prevLast := lastLoggedTime
	posthogClient = nil
	posthogInitialized = false
	lastLoggedTime = time.Time{}
	t.Cleanup(func() {
		posthogClient = prevClient
		posthogInitialized = prevInit
		lastLoggedTime = prevLast
	})
}

// isolateStoreDir points ~/.worldcli at a per-test temp dir so tests never read
// or write the developer's real store.
func isolateStoreDir(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func timestampPath(t *testing.T) string {
	t.Helper()
	p, err := getTimestampFilePath()
	require.NoError(t, err)
	return p
}

func writeTimestamp(t *testing.T, ts time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(timestampPath(t)), 0o700))
	require.NoError(t, os.WriteFile(timestampPath(t), []byte(ts.Format(time.DateOnly)), 0o644))
}

func readTimestamp(t *testing.T) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(timestampPath(t))
	if os.IsNotExist(err) {
		return "", false
	}
	require.NoError(t, err)
	return string(data), true
}

func TestPosthogInit_EmptyKey_LeavesClientUninitializedAndDoesNotStamp(t *testing.T) {
	isolateStoreDir(t)
	resetPosthogGlobals(t)

	PosthogInit("")

	assert.False(t, posthogInitialized, "empty API key must not initialize PostHog")
	assert.Nil(t, posthogClient, "no client should be created for an empty key")

	// Capturing after an empty-key init is a no-op even though the API key is
	// empty: this is the dev-build path, and it must not touch the dedupe file.
	PosthogCaptureEvent(RunningEvent, map[string]any{"version": "dev"})
	_, exists := readTimestamp(t)
	assert.False(t, exists, "dedupe file must not be written when PostHog is off")
}

func TestPosthogInit_DoesNotStampDedupeFile(t *testing.T) {
	isolateStoreDir(t)
	resetPosthogGlobals(t)

	posthogInitWithClient(&recordingPosthogClient{})

	assert.True(t, posthogInitialized)
	_, exists := readTimestamp(t)
	assert.False(t, exists, "init must not stamp the dedupe file; only a successful Running enqueue does")
	assert.True(t, lastLoggedTime.IsZero(), "with no prior file, lastLoggedTime stays zero")
}

func TestPosthogCaptureEvent_Running_FirstOfDay_EnqueuesAndStamps(t *testing.T) {
	isolateStoreDir(t)
	resetPosthogGlobals(t)
	fake := &recordingPosthogClient{}
	posthogInitWithClient(fake)

	PosthogCaptureEvent(RunningEvent, map[string]any{"version": "v1"})

	enqueued := fake.Enqueued()
	require.Len(t, enqueued, 1, "the Running event must be enqueued on the first of the day")
	assert.Equal(t, RunningEvent, enqueued[0].Event)
	assert.Equal(t, "v1", enqueued[0].Properties["version"])

	stamp, exists := readTimestamp(t)
	require.True(t, exists, "dedupe file must be stamped after a successful Running enqueue")
	assert.Equal(t, time.Now().Format(time.DateOnly), stamp)
	assert.True(t, isSameDay(lastLoggedTime, time.Now()), "in-memory dedupe clock must advance to today")
}

func TestPosthogCaptureEvent_Running_SameDay_SkipsDuplicateAndDoesNotRestamp(t *testing.T) {
	isolateStoreDir(t)
	resetPosthogGlobals(t)
	fake := &recordingPosthogClient{}
	posthogInitWithClient(fake)

	PosthogCaptureEvent(RunningEvent, map[string]any{"version": "v1"}) // enqueues + stamps
	require.Len(t, fake.Enqueued(), 1)
	firstStamp, _ := readTimestamp(t)

	// Second capture the same day (still the same process): the in-memory clock
	// suppresses the duplicate without re-enqueueing or rewriting the file.
	PosthogCaptureEvent(RunningEvent, map[string]any{"version": "v1"})
	assert.Len(t, fake.Enqueued(), 1, "same-day Running must be deduped (no second enqueue)")

	secondStamp, _ := readTimestamp(t)
	assert.Equal(t, firstStamp, secondStamp, "dedupe file must not be rewritten for a deduped event")
}

func TestPosthogCaptureEvent_Running_AcrossInvocations_DedupeIsFileDriven(t *testing.T) {
	// Two simulated processes sharing the same store dir (i.e. same machine,
	// same day). Process A captures and stamps; process B starts fresh, reads the
	// file, and dedupes -- exactly the cross-invocation contract.
	isolateStoreDir(t)
	resetPosthogGlobals(t)

	// Process A: first of day.
	a := &recordingPosthogClient{}
	posthogInitWithClient(a)
	PosthogCaptureEvent(RunningEvent, map[string]any{"version": "v1"})
	require.Len(t, a.Enqueued(), 1)

	// Process B: new process, same day. Intentionally do NOT close A: kong's
	// early exit / a crash skips PosthogClose. B must still see today's stamp.
	b := &recordingPosthogClient{}
	posthogInitWithClient(b) // re-reads the file -> lastLoggedTime = today
	assert.True(t, isSameDay(lastLoggedTime, time.Now()), "B must load today's stamp from the file")
	PosthogCaptureEvent(RunningEvent, map[string]any{"version": "v1"})
	assert.Empty(t, b.Enqueued(), "B must dedupe a same-day Running event using the file")
}

func TestPosthogCaptureEvent_Running_NextDay_EnqueuesAndRestamps(t *testing.T) {
	isolateStoreDir(t)
	resetPosthogGlobals(t)
	writeTimestamp(t, time.Now().AddDate(0, 0, -1)) // yesterday's stamp on disk

	fake := &recordingPosthogClient{}
	posthogInitWithClient(fake)
	assert.False(t, isSameDay(lastLoggedTime, time.Now()), "init must load yesterday's stamp")

	PosthogCaptureEvent(RunningEvent, map[string]any{"version": "v1"})
	require.Len(t, fake.Enqueued(), 1, "a new day must not be deduped")

	stamp, exists := readTimestamp(t)
	require.True(t, exists)
	assert.Equal(t, time.Now().Format(time.DateOnly), stamp, "file must be advanced to today")
}

func TestPosthogCaptureEvent_NonRunningEvent_DoesNotStampDedupe(t *testing.T) {
	isolateStoreDir(t)
	resetPosthogGlobals(t)
	fake := &recordingPosthogClient{}
	posthogInitWithClient(fake)
	prevLast := lastLoggedTime

	PosthogCaptureEvent(PostInstallationEvent, map[string]any{"version": "v1"})

	enqueued := fake.Enqueued()
	require.Len(t, enqueued, 1, "the PostInstallation event must still be enqueued")
	assert.Equal(t, PostInstallationEvent, enqueued[0].Event)

	_, exists := readTimestamp(t)
	assert.False(t, exists, "only Running events stamp the daily dedupe file")
	assert.Equal(t, prevLast, lastLoggedTime, "non-Running events must not move the dedupe clock")
}

func TestPosthogCaptureEvent_EnqueueError_DoesNotConsumeDay(t *testing.T) {
	isolateStoreDir(t)
	resetPosthogGlobals(t)
	writeTimestamp(t, time.Now().AddDate(0, 0, -1)) // yesterday
	fake := &recordingPosthogClient{enqueueErr: assertError("queue full")}
	posthogInitWithClient(fake)

	PosthogCaptureEvent(RunningEvent, map[string]any{"version": "v1"})

	assert.Empty(t, fake.Enqueued(), "a failed enqueue must not record an event")
	stamp, exists := readTimestamp(t)
	require.True(t, exists)
	assert.Equal(t, time.Now().AddDate(0, 0, -1).Format(time.DateOnly), stamp,
		"the stamp must stay on the prior day so the next invocation retries")
	assert.False(t, isSameDay(lastLoggedTime, time.Now()),
		"a failed enqueue must not advance the in-memory dedupe clock")
}

func TestPosthogClose_FlushesClientOnceAndResetsFlag(t *testing.T) {
	isolateStoreDir(t)
	resetPosthogGlobals(t)
	fake := &recordingPosthogClient{}
	posthogInitWithClient(fake)

	PosthogClose()
	assert.Equal(t, 1, fake.Closed(), "PosthogClose must flush the client exactly once")
	assert.False(t, posthogInitialized, "the initialized flag must be cleared")

	// Idempotent: a second close (e.g. the deferred one on main's normal return
	// path) must not call Close again or panic.
	PosthogClose()
	assert.Equal(t, 1, fake.Closed(), "PosthogClose must be idempotent")
	assert.False(t, posthogInitialized)
}

func TestFlushTelemetry_FlushesPostHogThenSentryInOrder(t *testing.T) {
	isolateStoreDir(t)
	resetPosthogGlobals(t)

	rec := &orderRecorder{}
	// PostHog side: fake client whose Close records into the shared recorder.
	posthogClient = &recordingPosthogClient{rec: rec}
	posthogInitialized = true

	// Sentry side: bind a client whose transport.Flush records into the same
	// recorder, then mark Sentry initialized so SentryFlush actually flushes.
	cleanup := bindTestSentry(t, &orderSentryTransport{rec: rec})
	defer cleanup()
	prevSentryInit := sentryInitialized
	sentryInitialized = true
	t.Cleanup(func() { sentryInitialized = prevSentryInit })

	FlushTelemetry()

	assert.Equal(t, []string{"posthog-close", "sentry-flush"}, rec.snapshot(),
		"FlushTelemetry must flush PostHog before Sentry, mirroring main's defer order")
	assert.False(t, posthogInitialized, "PostHog flag must be cleared")
	assert.False(t, sentryInitialized, "Sentry flag must be cleared")
}

func TestFlushTelemetry_NoopWhenUninitialized_DoesNotPanic(t *testing.T) {
	// Dev-build path: both sinks off. kong.Exit still calls FlushTelemetry, so it
	// must be a safe no-op even with a nil PostHog client.
	isolateStoreDir(t)
	resetPosthogGlobals(t) // posthogInitialized=false, posthogClient=nil
	prevSentryInit := sentryInitialized
	sentryInitialized = false
	t.Cleanup(func() { sentryInitialized = prevSentryInit })

	assert.NotPanics(t, func() { FlushTelemetry() })
}

func TestFlushTelemetry_DrainsBufferedEventWhenDefersSkipped(t *testing.T) {
	// This is the unit-level reproduction of the kong early-exit scenario: the
	// Running event is enqueued before kong.Parse, then kong os.Exits and main's
	// defers never run. The fix substitutes the defer with an explicit
	// FlushTelemetry() inside the kong.Exit hook. Here we enqueue, then call
	// FlushTelemetry directly (standing in for the hook, with NO deferred
	// PosthogClose), and assert the buffered event is handed to the client's
	// Close -- the only delivery mechanism -- so it is not stranded in memory.
	isolateStoreDir(t)
	resetPosthogGlobals(t)
	fake := &recordingPosthogClient{}
	posthogInitWithClient(fake)

	PosthogCaptureEvent(RunningEvent, map[string]any{"version": "v1"})
	require.Len(t, fake.Enqueued(), 1, "Running must be buffered before the early exit")

	// No deferred PosthogClose: simulate kong calling the hook and then os.Exit
	// skipping every main defer.
	FlushTelemetry()

	assert.Equal(t, 1, fake.Closed(), "FlushTelemetry must drain the buffered event via Close when defers are skipped")
	assert.False(t, posthogInitialized, "the hook must leave PostHog closed")
}

// assertError is a tiny sentinel error for enqueue/close failure injection.
type assertError string

func (e assertError) Error() string { return string(e) }
