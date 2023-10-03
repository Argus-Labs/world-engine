package interfaces

import "github.com/rs/zerolog"

type IWorldLogger interface {
	GetZeroLogger() *zerolog.Logger
	LogComponents(world IWorld, level zerolog.Level)
	LogSystem(world IWorld, level zerolog.Level)
	LogEntity(level zerolog.Level, entity IEntity, components []IComponentType)
	LogWorld(world IWorld, level zerolog.Level)
	CreateSystemLogger(systemName string) IWorldLogger
	CreateTraceLogger(traceId string) zerolog.Logger
	InjectLogger(logger *zerolog.Logger)
}
