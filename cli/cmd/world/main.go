package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
	"github.com/getsentry/sentry-go"

	"github.com/argus-labs/world-engine/cli/commands/root"
	pkgerrors "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/logger"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
)

// checkVerboseFlag checks if the verbose flag is present in command line arguments.
// Does not use kong to allow for early verbose mode setting.
func checkVerboseFlag() bool {
	for _, arg := range os.Args {
		if arg == "-v" || arg == "--verbose" {
			return true
		}
	}
	return false
}

// failureMessages gives one message per independent failure: kong's and
// errorspkg.JoinFailures' top-level joins are split, while a join under a wrap
// is one step's causes and stays one (multi-line) message.
func failureMessages(err error) []string {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var msgs []string
		for _, inner := range joined.Unwrap() {
			msgs = append(msgs, failureMessages(inner)...)
		}
		return msgs
	}
	return []string{err.Error()}
}

func main() {
	// Runs before anything else, so a hand-off leaves no telemetry or output from this binary.
	delegateToProjectTool(os.Args[1:])

	var cli root.Cmd

	// A failed command has to leave a non-zero status: scripts and CI read the exit code, and until now a
	// command could print its error and still exit 0 — a failed `world sdk generate` reported success.
	// Deferred first so it runs last, after the Sentry and PostHog flushes below; os.Exit skips whatever
	// defers remain, so anything that must flush has to be registered after this line, not before.
	exitCode := 0
	defer func() {
		// A panic must not be laundered into a clean exit. exitCode is set only where a command RETURNS
		// an error, so on a panic it is still 0 — and os.Exit runs during unwinding, terminating the
		// process before the runtime prints anything. The crash would be reported as a success with no
		// trace. Re-panicking hands it back to the normal crash path: full trace, non-zero status. The
		// flushes registered below still run first, since they were deferred after this.
		if r := recover(); r != nil {
			panic(r)
		}
		os.Exit(exitCode)
	}()

	if checkVerboseFlag() {
		logger.SetVerboseMode(true)
	}

	version := cli.GetVersion()
	env := cli.GetEnv()

	// Sentry initialization
	telemetry.SentryInit(root.SentryDsn, env, version)
	defer telemetry.SentryFlush()

	// Posthog Initialization
	telemetry.PosthogInit(root.PosthogAPIKey)
	defer telemetry.PosthogClose()

	// Capture event post installation
	if len(os.Args) > 1 && os.Args[1] == "post-installation" {
		telemetry.PosthogCaptureEvent(telemetry.PostInstallationEvent, map[string]any{
			"version": version,
		})
		return
	}

	// Capture event running
	telemetry.PosthogCaptureEvent(telemetry.RunningEvent, map[string]any{
		"version": version,
	})

	switch env {
	case "LOCAL":
		printer.Notificationln("World CLI Env: LOCAL")
	case "DEV":
		printer.Notificationln("World CLI Env: DEV")
	}

	ctx := kong.Parse(
		&cli,
		kong.Name("world"),
		kong.Description("World CLI: Your complete toolkit for World Engine development"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}),
		kong.Bind(&cli),
		kong.Vars{"version": version},
	)

	// kong is authoritative: it also accepts forms checkVerboseFlag misses, e.g. --verbose=true.
	logger.SetVerboseMode(cli.Verbose)

	realCtx := contextWithSigterm(context.Background())
	ctx.BindTo(realCtx, (*context.Context)(nil))
	err := ctx.Run()
	if err != nil {
		if logger.Verbose() {
			logger.Errors(err)
		}
		// Only print errors that should be shown to the user, non expected errors should be captured by Sentry
		if pkgerrors.ShouldPrint(err) {
			sentry.CaptureException(err)
			// One ✖ per failure, each with its whole chain: the outermost wrap
			// alone ("initial shard build") hides the cause.
			for _, msg := range failureMessages(err) {
				printer.Errorln(msg)
			}
		}
		exitCode = 1
	}
	// print log stack
	logger.PrintLogs()
	printer.NewLine(1)
}

// contextWithSigterm provides a context that automatically terminates when either the parent context is canceled or
// when a termination signal is received.
func contextWithSigterm(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	go func() {
		defer cancel()

		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

		select {
		case <-signalCh:
			printer.NewLine(1)
			printer.Infoln(textStyle.Render("Interrupt signal received. Terminating..."))
		case <-ctx.Done():
			printer.NewLine(1)
			printer.Infoln(textStyle.Render("Cancellation signal received. Terminating..."))
		}
	}()

	return ctx
}
