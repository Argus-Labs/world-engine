package telemetry

// FlushTelemetry flushes every telemetry sink in the order main's defers run
// them: PostHog first, then Sentry.
//
// It exists for kong's early-exit path: kong.Parse terminates the process via
// kong.Exit (whose default is [os.Exit]) on --help, --version, and parse
// errors. [os.Exit] skips main's defers, so the deferred PosthogClose -- the
// only thing that drains the in-memory PostHog event queue -- would never run,
// and the daily "Running" event captured before kong.Parse would be silently
// dropped (with the day's dedupe quota already consumed). Routing kong.Exit
// through FlushTelemetry runs those same flushes before the process
// terminates.
//
// The sinks are idempotent (PosthogClose and SentryFlush clear their
// initialized flags), so on the normal return path a later deferred
// PosthogClose/SentryFlush after FlushTelemetry is a harmless no-op.
func FlushTelemetry() {
	PosthogClose()
	SentryFlush()
}
