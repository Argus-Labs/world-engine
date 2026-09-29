package logger

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// Debug function.
func Debug(args ...any) {
	log.Debug().Timestamp().Msg(fmt.Sprint(args...))
}

// Debugln function.
func Debugln(args ...any) {
	log.Debug().Timestamp().Msg(fmt.Sprintln(args...))
}

// Debugf function.
func Debugf(format string, v ...any) {
	log.Debug().Timestamp().Msgf(format, v...)
}

// DebugWithFields function.
func DebugWithFields(msg string, kv map[string]any) {
	log.Debug().Timestamp().Fields(kv).Msg(msg)
}

// Info function.
func Info(args ...any) {
	log.Info().Timestamp().Msg(fmt.Sprint(args...))
}

// Infoln function.
func Infoln(args ...any) {
	log.Info().Timestamp().Msg(fmt.Sprintln(args...))
}

// Infof function.
func Infof(format string, v ...any) {
	log.Info().Timestamp().Msgf(format, v...)
}

// InfoWithFields function.
func InfoWithFields(msg string, kv map[string]any) {
	log.Info().Timestamp().Fields(kv).Msg(msg)
}

// Warn function.
func Warn(args ...any) {
	log.Warn().Timestamp().Msg(fmt.Sprint(args...))
}

// Warnln function.
func Warnln(args ...any) {
	log.Warn().Timestamp().Msg(fmt.Sprintln(args...))
}

// Warnf function.
func Warnf(format string, v ...any) {
	log.Warn().Timestamp().Msgf(format, v...)
}

// WarnWithFields function.
func WarnWithFields(msg string, kv map[string]any) {
	log.Warn().Timestamp().Fields(kv).Msg(msg)
}

// Error function.
func Error(args ...any) {
	log.Error().Timestamp().Msg(fmt.Sprint(args...))
}

// ErrorE function.
func ErrorE(err error) {
	log.Error().Timestamp().Err(err).Msg(err.Error())
}

// Errorln function.
func Errorln(args ...any) {
	log.Error().Timestamp().Msg(fmt.Sprintln(args...))
}

// Errorf function.
func Errorf(format string, v ...any) {
	log.Error().Timestamp().Msgf(format, v...)
}

// ErrorWithFields function.
func ErrorWithFields(msg string, kv map[string]any) {
	log.Error().Timestamp().Fields(kv).Msg(msg)
}

// Errors function to log errors package.
func Errors(err error) {
	log.Error().Timestamp().Msg(err.Error())
}

// Printf standard printf with debug mode validation.
func Printf(format string, v ...any) {
	if verboseMode.Load() {
		fmt.Printf(format, v...) //nolint:forbidigo // Need customer friendly output
	}
}

// Println standard println with debug mode validation.
func Println(v ...any) {
	if verboseMode.Load() {
		fmt.Println(v...) //nolint:forbidigo // Need customer friendly output
	}
}

// Print standard print with debug mode validation.
func Print(v ...any) {
	if verboseMode.Load() {
		fmt.Print(v...) //nolint:forbidigo // Need customer friendly output
	}
}
