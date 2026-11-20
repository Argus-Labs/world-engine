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
		opts := append(kongOptions(&cli, "test", exit),
			kong.Writers(&buf, io.Discard))
		k, err := kong.New(&cli, opts...)
		require.NoError(t, err, "kong.New must succeed for the CLI grammar")

		_, perr := k.Parse(args)
		k.FatalIfErrorf(perr) // terminates via k.Exit on a parse error
		returned = true
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
		name string
		args []string
		want int
	}{
		{"help", []string{"--help"}, 0},
		{"version", []string{"--version"}, 0},
		{"bare", nil, 80}, // kong.exitUsageError: no command given
		{"parse-error", []string{"--definitely-not-a-flag"}, 80},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called, code, stdout, returned := parseViaKong(t, tc.args)

			require.True(t, called, "kong.Exit override must be invoked on %s", tc.name)
			require.False(t, returned, "kong must terminate before returning on an early-exit path")
			assert.Equal(t, tc.want, code, "%s must terminate with the expected exit code", tc.name)
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
