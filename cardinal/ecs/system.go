package ecs

import "github.com/rs/zerolog"

type RegisteredSystem func(*World, *TransactionQueue) error
type System func(*World, *TransactionQueue, *zerolog.Logger) error
