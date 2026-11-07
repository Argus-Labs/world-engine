package logger

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout captures [os.Stdout] output produced by fn and returns it as a string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w //nolint:reassign // need to capture stdout

	fn()

	_ = w.Close()
	os.Stdout = orig //nolint:reassign // need to capture stdout
	buf, _ := io.ReadAll(r)
	_ = r.Close()
	return string(buf)
}

func TestError_WritesToBuffer(t *testing.T) {
	// Reset buffer to isolate test
	logBuffer.Reset()

	Error("error message")

	out := logBuffer.String()
	assert.Contains(t, out, "error message")
}

func TestErrors_LogErrors(t *testing.T) {
	logBuffer.Reset()
	err := errors.New("boom")
	Errors(err)
	out := logBuffer.String()
	assert.Contains(t, out, "boom")
}

func TestPrintLogs_NoOutputWhenNotVerboseOrEmpty(t *testing.T) {
	prev := Verbose()
	defer SetVerboseMode(prev)

	// Verbose off: no output
	SetVerboseMode(false)
	logBuffer.Reset()
	out := captureStdout(t, func() { PrintLogs() })
	assert.Empty(t, out)

	// Verbose on but buffer empty: no output
	SetVerboseMode(true)
	logBuffer.Reset()
	out = captureStdout(t, func() { PrintLogs() })
	assert.Empty(t, out)
}

func TestPrintLogs_PrintsBufferedLogsWhenVerbose(t *testing.T) {
	prev := Verbose()
	defer SetVerboseMode(prev)
	SetVerboseMode(true)

	logBuffer.Reset()
	Error("hello logs")

	out := captureStdout(t, func() {
		PrintLogs()
	})

	assert.Contains(t, out, "----- Log -----")
	assert.Contains(t, out, "hello logs")
}

// Note: We intentionally skip invoking Fatal* helpers in tests, because zerolog's
// Fatal always exits the process in this version (no Exit override available),
// which would terminate the test process and prevent coverage recording.

// ---------------------------------------------------------------------------
// zerologHandler tests
// ---------------------------------------------------------------------------

func TestZerologHandler_Enabled_RespectsVerboseMode(t *testing.T) {
	prev := Verbose()
	defer SetVerboseMode(prev)

	h := zerologHandler{}

	SetVerboseMode(true)
	assert.True(t, h.Enabled(context.Background(), slog.LevelDebug))
	assert.True(t, h.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, h.Enabled(context.Background(), slog.LevelWarn))
	assert.True(t, h.Enabled(context.Background(), slog.LevelError))

	SetVerboseMode(false)
	assert.False(t, h.Enabled(context.Background(), slog.LevelDebug))
	assert.False(t, h.Enabled(context.Background(), slog.LevelInfo))
	assert.False(t, h.Enabled(context.Background(), slog.LevelWarn))
	assert.False(t, h.Enabled(context.Background(), slog.LevelError))
}

func TestZerologHandler_Handle_RoutesLevels(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
		msg   string
	}{
		{"debug", slog.LevelDebug, "debug-via-handler"},
		{"info", slog.LevelInfo, "info-via-handler"},
		{"warn", slog.LevelWarn, "warn-via-handler"},
		{"error", slog.LevelError, "error-via-handler"},
	}

	h := zerologHandler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logBuffer.Reset()
			rec := slog.NewRecord(time.Now(), tt.level, tt.msg, 0)
			err := h.Handle(context.Background(), rec)
			require.NoError(t, err)
			assert.Contains(t, logBuffer.String(), tt.msg)
		})
	}
}

func TestZerologHandler_Handle_IncludesRecordAttrs(t *testing.T) {
	logBuffer.Reset()
	h := zerologHandler{}

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "with-attrs", 0)
	rec.AddAttrs(slog.String("fruit", "apple"), slog.Int("count", 3))
	err := h.Handle(context.Background(), rec)
	require.NoError(t, err)

	out := logBuffer.String()
	assert.Contains(t, out, "with-attrs")
	assert.Contains(t, out, "apple")
	assert.Contains(t, out, "3")
}

func TestZerologHandler_WithAttrs_MergesAttributes(t *testing.T) {
	logBuffer.Reset()
	base := zerologHandler{}
	child := base.WithAttrs([]slog.Attr{slog.String("env", "test")}).(zerologHandler)

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "merged-test", 0)
	err := child.Handle(context.Background(), rec)
	require.NoError(t, err)

	assert.Contains(t, logBuffer.String(), "test")
	assert.Empty(t, base.attrs, "original handler must not be mutated")
}

func TestZerologHandler_WithAttrs_Chains(t *testing.T) {
	logBuffer.Reset()
	h := zerologHandler{}
	h1 := h.WithAttrs([]slog.Attr{slog.String("a", "1")}).(zerologHandler)
	h2 := h1.WithAttrs([]slog.Attr{slog.String("b", "2")}).(zerologHandler)

	assert.Len(t, h2.attrs, 2)
	assert.Equal(t, "a", h2.attrs[0].Key)
	assert.Equal(t, "b", h2.attrs[1].Key)
	assert.Len(t, h1.attrs, 1, "first child must not be mutated by second WithAttrs")
}

func TestZerologHandler_WithGroup_ReturnsSelf(t *testing.T) {
	h := zerologHandler{attrs: []slog.Attr{slog.String("x", "y")}}
	g := h.WithGroup("ignored")
	assert.Equal(t, h, g)
}

func TestSlog_ReturnsNonNilLogger(t *testing.T) {
	l := Slog()
	require.NotNil(t, l)
}

func TestSlog_Integration_WritesToBuffer(t *testing.T) {
	prev := Verbose()
	defer SetVerboseMode(prev)
	SetVerboseMode(true)

	logBuffer.Reset()
	l := Slog()
	l.InfoContext(context.Background(), "slog-integration", "key", "val")

	out := logBuffer.String()
	assert.Contains(t, out, "slog-integration")
	assert.Contains(t, out, "val")
}

func TestSlog_Integration_SilentWhenNotVerbose(t *testing.T) {
	prev := Verbose()
	defer SetVerboseMode(prev)
	SetVerboseMode(false)

	logBuffer.Reset()
	l := Slog()
	l.InfoContext(context.Background(), "should-not-appear")

	assert.Empty(t, logBuffer.String())
}
