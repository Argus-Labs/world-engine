package telemetry

import (
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
)

var (
	//nolint:gochecknoglobals // Global flag for controlling sentry initialization
	sentryInitialized bool
)

// SentryInit initialize sentry.
func SentryInit(sentryDsn string, env string, appVersion string) {
	// Skip initialization if DSN is empty or placeholder
	if sentryDsn == "" || sentryDsn == "default_null" {
		return
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              sentryDsn,
		EnableTracing:    true,
		TracesSampleRate: 1.0,
		AttachStacktrace: true,
		Environment:      env,
		Release:          fmt.Sprintf("world-cli@%s", appVersion),
	})
	if err != nil {
		log.Err(err).Msg("Cannot initialize sentry. \r\n" +
			" Please check your INFISICAL_WORLD_CLI_SENTRY_KEY in your GitHub Actions secrets for World CLI repository. \r\n" +
			" If you are running locally, you can ignore this message.\r\n")
		return
	}

	sentryInitialized = true
}

func SentryFlush() {
	if sentryInitialized {
		err := recover()
		if err != nil {
			sentry.CurrentHub().Recover(err)
		}

		// Flush buffered events before the program terminates.
		// Set the timeout to the maximum duration the program can afford to wait.
		sentry.Flush(5 * time.Second)
		sentryInitialized = false
		if err != nil {
			// Re-raise the recovered panic so unwinding continues to main()'s top-level
			// handler, which owns the crash path (non-zero exit code and a stack trace).
			// Without this, the recover() above would own the panic and main's later
			// handler would observe recover()==nil and call os.Exit(0), laundering an
			// unrecovered crash into a successful exit — the very failure the sibling
			// defer in main() was added to prevent. Recover() and Flush() must run
			// first (as they do above) so the Sentry event is delivered before the
			// re-panic terminates the process.
			panic(err)
		}
	}
}
