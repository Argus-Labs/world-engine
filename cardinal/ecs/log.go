package ecs

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type Logger struct {
	*zerolog.Logger
}

func (_ *Logger) loadComponentIntoArrayLogger(component component.IComponentType, arrayLogger *zerolog.Array) *zerolog.Array {
	dictLogger := zerolog.Dict()
	dictLogger = dictLogger.Int("component_id", int(component.ID()))
	dictLogger = dictLogger.Str("component_name", component.Name())
	return arrayLogger.Dict(dictLogger)
}

func (l *Logger) loadComponentsToEvent(zeroLoggerEvent *zerolog.Event, world *World) *zerolog.Event {
	zeroLoggerEvent.Int("total_components", len(*world.GetComponents()))
	arrayLogger := zerolog.Arr()
	for _, _component := range *world.GetComponents() {
		arrayLogger = l.loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	return zeroLoggerEvent.Array("components", arrayLogger)
}

func (_ *Logger) loadSystemIntoArrayLogger(world *World, registeredSystemIndex int, arrayLogger *zerolog.Array) *zerolog.Array {
	return arrayLogger.Str(world.SystemNames[registeredSystemIndex])
}

func (l *Logger) loadSystemIntoEvent(zeroLoggerEvent *zerolog.Event, world *World) *zerolog.Event {
	zeroLoggerEvent.Int("total_systems", len(*world.GetSystems()))
	arrayLogger := zerolog.Arr()
	for index, _ := range *world.GetSystems() {
		arrayLogger = l.loadSystemIntoArrayLogger(world, index, arrayLogger)
	}
	return zeroLoggerEvent.Array("systems", arrayLogger)
}

func (l *Logger) loadEntityIntoEvent(zeroLoggerEvent *zerolog.Event, world *World, entityID storage.EntityID) (*zerolog.Event, error) {
	entity, err := world.Entity(entityID)
	if err != nil {
		return nil, err
	}

	archetype := entity.Archetype(world)
	arrayLogger := zerolog.Arr()
	for _, _component := range archetype.Layout().Components() {
		arrayLogger = l.loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	zeroLoggerEvent.Array("components", arrayLogger)
	zeroLoggerEvent.Int("entity_id", int(entityID))
	return zeroLoggerEvent.Int("archetype_id", int(entity.Loc.ArchID)), nil
}

func (l *Logger) LogComponents(world *World, level zerolog.Level) {
	zeroLoggerEvent := l.WithLevel(level)
	zeroLoggerEvent = l.loadComponentsToEvent(zeroLoggerEvent, world)
	zeroLoggerEvent.Send()
}

func (l *Logger) LogSystem(world *World, level zerolog.Level) {
	zeroLoggerEvent := l.WithLevel(level)
	zeroLoggerEvent = l.loadSystemIntoEvent(zeroLoggerEvent, world)
	zeroLoggerEvent.Send()
}

func (l *Logger) LogEntity(world *World, level zerolog.Level, entityID storage.EntityID) error {
	zeroLoggerEvent := l.WithLevel(level)
	var err error = nil
	zeroLoggerEvent, err = l.loadEntityIntoEvent(zeroLoggerEvent, world, entityID)
	if err != nil {
		return err
	}
	zeroLoggerEvent.Send()
	return nil
}

func (l *Logger) LogWorld(world *World, level zerolog.Level) {
	zeroLoggerEvent := l.WithLevel(level)
	zeroLoggerEvent = l.loadComponentsToEvent(zeroLoggerEvent, world)
	zeroLoggerEvent = l.loadSystemIntoEvent(zeroLoggerEvent, world)
	zeroLoggerEvent.Send()
}

func (l *Logger) CreateSystemLogger(systemName string) Logger {
	zeroLogger := l.Logger.With().
		Str("system", systemName).Logger()
	return Logger{
		&zeroLogger,
	}
}

func (l *Logger) CreateTraceLogger(traceId string) zerolog.Logger {
	return l.Logger.With().
		Str("trace_id", traceId).
		Logger()
}
