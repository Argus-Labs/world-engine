//nolint:reassign,forbidigo // customizing zerolog globals is safe and intentional for this CLI
package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	DefaultTimeFormat           = "15:04:05.000"
	DefaultCallerSkipFrameCount = 3 // set to 3 because logger wrapped in logger.go

	NoColor   = true
	UseCaller = false // for developer, if you want to expose line of code of caller
)

// syncBuffer is a thread-safe wrapper around bytes.Buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

func (sb *syncBuffer) Reset() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.buf.Reset()
}

var _ io.Writer = (*syncBuffer)(nil)

var (
	//nolint:gochecknoglobals // Global buffer for collecting logs in memory
	logBuffer = &syncBuffer{}

	//nolint:gochecknoglobals // Global flag for controlling verbose logging output
	verboseMode atomic.Bool

	//nolint:gochecknoglobals // Structured logger for packages that accept *slog.Logger
	slogLogger *slog.Logger
)

//nolint:gochecknoinits // Common package init, should self init as it shouldn't have dependencies..
func init() {
	var (
		lgr zerolog.Logger
	)

	zerolog.TimeFieldFormat = DefaultTimeFormat
	zerolog.CallerSkipFrameCount = DefaultCallerSkipFrameCount

	var writers zerolog.LevelWriter
	consoleWriter := zerolog.ConsoleWriter{
		Out:        logBuffer,
		NoColor:    NoColor,
		TimeFormat: DefaultTimeFormat,
	}
	writers = zerolog.MultiLevelWriter(consoleWriter)

	lgr = zerolog.New(writers)

	if UseCaller {
		lgr = lgr.With().Caller().Logger()
	}

	log.Logger = lgr

	slogLogger = slog.New(zerologHandler{})
}

// zerologHandler is an slog.Handler that routes records through the package-level
// zerolog logger so output format matches the rest of the CLI.
type zerologHandler struct {
	attrs []slog.Attr
}

func (h zerologHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return verboseMode.Load()
}

func (h zerologHandler) Handle(_ context.Context, r slog.Record) error {
	var evt *zerolog.Event
	switch {
	case r.Level >= slog.LevelError:
		evt = log.Error()
	case r.Level >= slog.LevelWarn:
		evt = log.Warn()
	case r.Level >= slog.LevelInfo:
		evt = log.Info()
	default:
		evt = log.Debug()
	}
	evt = evt.Timestamp()
	for _, a := range h.attrs {
		evt = evt.Interface(a.Key, a.Value.Any())
	}
	r.Attrs(func(a slog.Attr) bool {
		evt = evt.Interface(a.Key, a.Value.Any())
		return true
	})
	evt.Msg(r.Message)
	return nil
}

func (h zerologHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	return zerologHandler{attrs: append(merged, attrs...)}
}

func (h zerologHandler) WithGroup(_ string) slog.Handler { return h }

// Slog returns a *slog.Logger that routes through zerolog and is
// gated on Verbose().
func Slog() *slog.Logger {
	return slogLogger
}

// Verbose reports whether verbose/debug logging is enabled.
func Verbose() bool {
	return verboseMode.Load()
}

func SetVerboseMode(verbose bool) {
	verboseMode.Store(verbose)
}

// PrintLogs print all stacked log.
func PrintLogs() {
	if verboseMode.Load() {
		// Extract the logs from the buffer and print them
		logs := logBuffer.String()
		if len(logs) > 0 {
			fmt.Println()
			fmt.Println("----- Log -----")
			fmt.Println(logs)
		}
	}
}
