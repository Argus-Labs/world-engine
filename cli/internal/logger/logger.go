package logger

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// Error function.
func Error(args ...any) {
	log.Error().Timestamp().Msg(fmt.Sprint(args...))
}

// Errors function to log errors package.
func Errors(err error) {
	log.Error().Timestamp().Msg(err.Error())
}
