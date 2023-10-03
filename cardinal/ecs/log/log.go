package log

import (
	"fmt"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type Logger struct {
	Logger *zerolog.Logger
}

func (_ *Logger) loadComponentIntoArrayLogger(component interfaces.IComponentType, arrayLogger *zerolog.Array) *zerolog.Array {
	dictLogger := zerolog.Dict()
	dictLogger = dictLogger.Int("component_id", int(component.ID()))
	dictLogger = dictLogger.Str("component_name", component.Name())
	return arrayLogger.Dict(dictLogger)
}

func (l *Logger) loadComponentsToEvent(zeroLoggerEvent *zerolog.Event, target interfaces.IWorld) *zerolog.Event {
	zeroLoggerEvent.Int("total_components", len(target.GetComponents()))
	arrayLogger := zerolog.Arr()
	for _, _component := range target.GetComponents() {
		arrayLogger = l.loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	return zeroLoggerEvent.Array("components", arrayLogger)
}

func (_ *Logger) loadSystemIntoArrayLogger(name string, arrayLogger *zerolog.Array) *zerolog.Array {
	return arrayLogger.Str(name)
}

func (l *Logger) loadSystemIntoEvent(zeroLoggerEvent *zerolog.Event, world interfaces.IWorld) *zerolog.Event {
	zeroLoggerEvent.Int("total_systems", len(world.GetSystemNames()))
	arrayLogger := zerolog.Arr()
	for _, name := range world.GetSystemNames() {
		arrayLogger = l.loadSystemIntoArrayLogger(name, arrayLogger)
	}
	return zeroLoggerEvent.Array("systems", arrayLogger)
}

func (l *Logger) loadEntityIntoEvent(zeroLoggerEvent *zerolog.Event, entity interfaces.IEntity, components []interfaces.IComponentType) (*zerolog.Event, error) {
	arrayLogger := zerolog.Arr()
	for _, _component := range components {
		arrayLogger = l.loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	zeroLoggerEvent.Array("components", arrayLogger)
	zeroLoggerEvent.Int("entity_id", int(entity.EntityID()))
	return zeroLoggerEvent.Int("archetype_id", int(entity.GetArchID())), nil
}

// LogComponents logs all component info related to the world
func (l *Logger) LogComponents(world interfaces.IWorld, level zerolog.Level) {
	zeroLoggerEvent := l.Logger.WithLevel(level)
	zeroLoggerEvent = l.loadComponentsToEvent(zeroLoggerEvent, world)
	zeroLoggerEvent.Send()
}

// LogSystem logs all system info related to the world
func (l *Logger) LogSystem(world interfaces.IWorld, level zerolog.Level) {
	zeroLoggerEvent := l.Logger.WithLevel(level)
	zeroLoggerEvent = l.loadSystemIntoEvent(zeroLoggerEvent, world)
	zeroLoggerEvent.Send()
}

// LogEntity logs entity info given an entityID
func (l *Logger) LogEntity(level zerolog.Level, entity interfaces.IEntity, components []interfaces.IComponentType) {
	zeroLoggerEvent := l.Logger.WithLevel(level)
	var err error = nil
	zeroLoggerEvent, err = l.loadEntityIntoEvent(zeroLoggerEvent, entity, components)
	if err != nil {
		l.Logger.Err(err).Msg(fmt.Sprintf("Error in Logger when retrieving entity with id %d", entity.EntityID()))
	} else {
		zeroLoggerEvent.Send()
	}
}

func (l *Logger) GetZeroLogger() *zerolog.Logger {
	return l.Logger
}

// LogWorld Logs everything about the world (components and Systems)
func (l *Logger) LogWorld(world interfaces.IWorld, level zerolog.Level) {
	zeroLoggerEvent := l.Logger.WithLevel(level)
	zeroLoggerEvent = l.loadComponentsToEvent(zeroLoggerEvent, world)
	zeroLoggerEvent = l.loadSystemIntoEvent(zeroLoggerEvent, world)
	zeroLoggerEvent.Send()
}

// CreateSystemLogger creates a Sub Logger with the entry {"system" : systemName}
func (l *Logger) CreateSystemLogger(systemName string) interfaces.IWorldLogger {
	zeroLogger := l.Logger.With().
		Str("system", systemName).Logger()
	res := Logger{
		&zeroLogger,
	}
	return &res
}

// CreateTraceLogger Creates a trace Logger. Using a single id you can use this Logger to follow and log a data path.
func (l *Logger) CreateTraceLogger(traceId string) zerolog.Logger {
	return l.Logger.With().
		Str("trace_id", traceId).
		Logger()
}

func (l *Logger) InjectLogger(logger *zerolog.Logger) {
	l.Logger = logger
}
