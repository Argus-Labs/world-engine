package telemetry

// FlushTelemetry flushes every telemetry sink in the order main's defers run
// them: PostHog first, then Sentry.
//
// It is the flush path for kong's early-exit hook: kong.Parse terminates
// --help/--version/parse errors via kong.Exit (whose default is [os.Exit]),
// which skips main's defers and would drop the buffered daily "Running"
// event. The in-package tests also call it directly to assert that order and
// the uninitialized no-op without standing up kong.
func FlushTelemetry() {
	PosthogClose()
	SentryFlush()
}
