package logtail

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestFormatShardLine_JSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantParts []string // substrings the visible (ANSI-stripped) output must contain
		notParts  []string // substrings that must NOT appear
	}{
		{
			name: "zerolog info line is pretty-printed",
			raw:  `{"level":"info","component":"cardinal.shard","time":"2026-06-01T02:31:07Z","caller":"github.com/argus-labs/world-engine@v0.11.2/pkg/cardinal/cardinal.go:161","message":"starting core shard loop"}`,
			wantParts: []string{
				"[gameplay]",
				"02:31:07",
				"INF",
				"starting core shard loop",
				"component=cardinal.shard",
			},
			notParts: []string{
				"caller=",
				"github.com/argus-labs",
				`"level":"info"`, // raw JSON should not leak through
			},
		},
		{
			name: "warn level is abbreviated to zerolog's 3-char WRN",
			raw:  `{"level":"warn","time":"2026-06-01T02:31:08Z","message":"slow tick"}`,
			wantParts: []string{
				"WRN", // zerolog's ConsoleWriter abbreviation for warn
				"slow tick",
				"02:31:08",
			},
		},
		{
			name: "error keeps the error field in extras",
			raw:  `{"level":"error","error":"context canceled","time":"2026-06-01T02:31:09Z","message":"failed running world"}`,
			wantParts: []string{
				"ERR",
				"failed running world",
				`error="context canceled"`, // zerolog quotes values containing spaces
			},
		},
		{
			name: "extras are sorted alphabetically",
			raw:  `{"level":"info","time":"2026-06-01T02:31:10Z","message":"loaded","zeta":1,"alpha":2}`,
			wantParts: []string{
				"alpha=2 zeta=1", // alpha before zeta
			},
		},
		{
			name: "no extras → no trailing key=value section",
			raw:  `{"level":"info","time":"2026-06-01T02:31:11Z","message":"plain"}`,
			notParts: []string{
				"=",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ansi.Strip(formatShardLine("gameplay", "#00FF00", tt.raw))
			for _, want := range tt.wantParts {
				require.Contains(t, got, want, "missing %q in %q", want, got)
			}
			for _, notWant := range tt.notParts {
				require.NotContains(t, got, notWant, "unexpected %q in %q", notWant, got)
			}
		})
	}
}

func TestFormatShardLine_NonJSON_PassesThrough(t *testing.T) {
	t.Parallel()
	raw := "to reproduce: TEST_SEED=0x18b496a8d6c33373"
	got := ansi.Strip(formatShardLine("gameplay", "#00FF00", raw))
	require.Equal(t, "[gameplay] "+raw, got)
}

func TestFormatShardLine_StripsTrailingNewline(t *testing.T) {
	t.Parallel()
	got := ansi.Strip(formatShardLine("gameplay", "#00FF00", "plain text\n"))
	require.Equal(t, "[gameplay] plain text", got)
}

func TestShortTime(t *testing.T) {
	t.Parallel()
	require.Equal(t, "02:31:07", shortTime("2026-06-01T02:31:07Z"))
	require.Equal(t, "02:31:07.123", shortTime("2026-06-01T02:31:07.123Z"))
	require.Equal(t, "02:31:07.123", shortTime("2026-06-01T02:31:07.123456789Z")) // truncated to ms
	require.Equal(t, "02:31:07.123", shortTime("2026-06-01T02:31:07.123+07:00"))  // tz dropped, ms kept
	require.Equal(t, "02:31:07", shortTime("2026-06-01T02:31:07"))                // no tz, no ms
	require.Equal(t, "weird", shortTime("weird"))                                 // non-standard → passthrough
	require.Equal(t, "", shortTime(""))
}
