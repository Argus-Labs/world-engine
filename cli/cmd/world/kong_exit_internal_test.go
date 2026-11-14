package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/commands/root"
)

// kongExitSentinel is panicked by the test's kong.Exit override to stop kong
// from proceeding past the termination point (the real override calls
// [os.Exit], which we cannot do in a test). Recovering it proves the override
// ran, and only the override, before kong returned.
const kongExitSentinel = "kong-exit-override-sentinel"

// kongOptionsForTest mirrors the option set main passes to kong.Parse, plus a
// kong.Exit override and quiet writers. Keeping it aligned with main ensures
// the test exercises the same kong behavior the production wiring relies on.
func kongOptionsForTest(exit func(int), stdout io.Writer) []kong.Option {
	var cli root.Cmd
	return []kong.Option{
		kong.Name("world"),
		kong.Description("World CLI: Your complete toolkit for World Engine development"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true, Summary: true}),
		kong.Bind(&cli),
		kong.Vars{"version": "test"},
		kong.Writers(stdout, io.Discard),
		kong.Exit(exit),
	}
}

// parseViaKong drives kong the way the package-level kong.Parse does
// (kong.New -> Kong.Parse -> FatalIfErrorf) but with explicit args, so the
// test does not depend on [os.Args]. It reports whether the kong.Exit override
// was called, with what code, what stdout was produced, and whether kong
// returned normally (it must not on early-exit paths).
func parseViaKong(t *testing.T, args []string) (bool, int, string, bool) {
	t.Helper()
	var cli root.Cmd
	var buf bytes.Buffer
	var (
		called   bool
		code     int
		returned bool
	)
	exit := func(c int) {
		called = true
		code = c
		panic(kongExitSentinel)
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				if r != kongExitSentinel {
					t.Fatalf("unexpected panic from kong: %v", r)
				}
			}
		}()
		k, err := kong.New(&cli, kongOptionsForTest(exit, &buf)...)
		require.NoError(t, err, "kong.New must succeed for the CLI grammar")

		ctx, perr := k.Parse(args)
		k.FatalIfErrorf(perr) // terminates via k.Exit on a parse error
		returned = true
		_ = ctx
	}()
	return called, code, buf.String(), returned
}

// TestKongEarlyExit_RoutesThroughExitOverride is the regression guard for the
// fix's central assumption: kong terminates --help, --version, and parse
// errors by calling kong.Exit (default [os.Exit]), which skips every defer in
// main. The fix routes that hook through telemetry.FlushTelemetry before
// [os.Exit]; this test pins that kong actually calls the hook on those paths.
// If a kong upgrade changes that, the override would silently stop running and
// the daily Running event would be dropped again.
func TestKongEarlyExit_RoutesThroughExitOverride(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantZero bool
	}{
		{"help", []string{"--help"}, true},
		{"version", []string{"--version"}, true},
		{"parse-error", []string{"--definitely-not-a-flag"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called, code, stdout, returned := parseViaKong(t, tc.args)

			require.True(t, called, "kong.Exit override must be invoked on %s", tc.name)
			require.False(t, returned, "kong must terminate before returning on an early-exit path")
			if tc.wantZero {
				assert.Equal(t, 0, code, "%s must terminate with exit code 0", tc.name)
			} else {
				assert.NotZero(t, code, "%s must terminate with a non-zero exit code", tc.name)
			}
			// The terminating output must still be produced before the exit, so
			// --help/--version remain useful to users on the post-fix path.
			assert.NotEmpty(t, stdout, "kong must print help/version/usage before calling kong.Exit")
		})
	}
}

// TestKongValidParse_DoesNotInvokeExitOverride confirms the override only fires
// on early-exit paths: a valid parse must return normally so main's normal
// defer chain (the real flush path) runs instead.
func TestKongValidParse_DoesNotInvokeExitOverride(t *testing.T) {
	called, _, _, returned := parseViaKong(t, []string{"doctor"})

	assert.False(t, called, "a valid parse must not call kong.Exit")
	assert.True(t, returned, "a valid parse must return normally so main's defers run")
}
