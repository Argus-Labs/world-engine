package logtail

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/rs/zerolog"

	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

// dimGray colors the timestamp. It's the one color we still pick by hand;
// zerolog's ConsoleWriter owns level + field coloring.
const dimGray = "#888888"

// consoleNoColor mirrors lipgloss's own color detection (true when stdout isn't
// a color terminal) so the zerolog-rendered portion drops ANSI in exactly the
// situations style.ForegroundPrint does. Detected once at startup, same pattern
// as the printer package.
//
//nolint:gochecknoglobals // read-only, initialized once.
var consoleNoColor = lipgloss.DefaultRenderer().ColorProfile() == termenv.Ascii

// formatShardLine takes one raw line from a shard pod and returns it ready to
// print. zerolog JSON lines are rendered with zerolog's own ConsoleWriter — the
// same "HH:MM:SS LVL message  k=v" layout the shards print locally — so we don't
// reimplement level coloring, field sorting, or caller suppression. Anything
// that isn't a zerolog JSON object passes through unchanged with just the
// shard prefix.
//
// The caller field is dropped (it's always a long module path that crowds out
// the message) and the timestamp is trimmed to HH:MM:SS(.mmm) by shortTime.
func formatShardLine(label, labelColor, raw string) string {
	raw = strings.TrimRight(raw, "\n")
	prefix := "[" + style.ForegroundPrint(label, labelColor) + "] "
	if rendered, ok := renderZerolog(raw); ok {
		return prefix + rendered
	}
	return prefix + raw
}

// renderZerolog pretty-prints a single zerolog JSON line via ConsoleWriter. ok
// is false when raw isn't a JSON object, in which case the caller passes the
// line through verbatim.
func renderZerolog(raw string) (string, bool) {
	// ConsoleWriter accepts a bare `null` and renders it as `???`.
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return "", false
	}
	var buf bytes.Buffer
	cw := zerolog.ConsoleWriter{
		Out:     &buf,
		NoColor: consoleNoColor,
		// Drop the caller part; keep time → level → message. Leftover fields are
		// appended after, sorted, by ConsoleWriter itself.
		PartsOrder: []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
			zerolog.MessageFieldName,
		},
		FormatTimestamp: func(i any) string {
			if i == nil {
				return "" // no time field → omit the part entirely
			}
			// zerolog decodes with UseNumber, so a numeric (unix) time arrives
			// as a json.Number, not a string — fall back to its literal form so
			// the timestamp still renders instead of vanishing.
			s, ok := i.(string)
			if !ok {
				s = fmt.Sprintf("%v", i)
			}
			t := shortTime(s)
			if t == "" || consoleNoColor {
				return t
			}
			return style.ForegroundPrint(t, dimGray)
		},
	}
	if _, err := cw.Write([]byte(raw)); err != nil {
		return "", false
	}
	return strings.TrimRight(buf.String(), "\n"), true
}

// shortTime extracts HH:MM:SS.mmm from an RFC3339Nano timestamp, keeping
// milliseconds when present (useful for diagnosing tick timing). Trailing
// timezone marker is dropped. Non-standard inputs pass through.
//
// Examples:
//
//	"2026-06-01T02:31:07Z"            → "02:31:07"
//	"2026-06-01T02:31:07.123Z"        → "02:31:07.123"
//	"2026-06-01T02:31:07.123456789Z"  → "02:31:07.123"   // truncated to ms
//	"2026-06-01T02:31:07.123+07:00"   → "02:31:07.123"
func shortTime(t string) string {
	if len(t) < 19 || t[10] != 'T' {
		return t
	}
	hms := t[11:19] // "02:31:07"
	rest := t[19:]  // "", "Z", ".123Z", ".123456789Z", ".123+07:00", ...
	if len(rest) == 0 || rest[0] != '.' {
		return hms
	}
	// Capture up to 3 fractional digits (millisecond precision).
	frac := rest[1:]
	n := 0
	for n < len(frac) && n < 3 && frac[n] >= '0' && frac[n] <= '9' {
		n++
	}
	if n == 0 {
		return hms
	}
	return hms + "." + frac[:n]
}
