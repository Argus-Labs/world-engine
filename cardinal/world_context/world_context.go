package world_context

import (
	"github.com/rs/zerolog"
)

type WorldContext interface {
	CurrentTick() uint64
	Logger() *zerolog.Logger
	IsReadOnly() bool
}
