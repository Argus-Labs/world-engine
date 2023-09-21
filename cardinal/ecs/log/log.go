package log

import (
	"fmt"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

type Loggable interface {
	GetComponents() []component.IComponentType
	GetSystemNames() []string
}

type Logger struct {
	*zerolog.Logger
}

func (_ *Logger) loadComponentIntoArrayLogger(component component.IComponentType, arrayLogger *zerolog.Array) *zerolog.Array {
	dictLogger := zerolog.Dict()
	dictLogger = dictLogger.Int("component_id", int(component.ID()))
	dictLogger = dictLogger.Str("component_name", component.Name())
	return arrayLogger.Dict(dictLogger)
}

func (l *Logger) loadComponentsToEvent(zeroLoggerEvent *zerolog.Event, target Loggable) *zerolog.Event {
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

func (l *Logger) loadSystemIntoEvent(zeroLoggerEvent *zerolog.Event, target Loggable) *zerolog.Event {
	zeroLoggerEvent.Int("total_systems", len(target.GetSystemNames()))
	arrayLogger := zerolog.Arr()
	for _, name := range target.GetSystemNames() {
		arrayLogger = l.loadSystemIntoArrayLogger(name, arrayLogger)
	}
	return zeroLoggerEvent.Array("systems", arrayLogger)
}

func (l *Logger) loadEntityIntoEvent(zeroLoggerEvent *zerolog.Event, entity entity.Entity, components []component.IComponentType) (*zerolog.Event, error) {
	arrayLogger := zerolog.Arr()
	for _, _component := range components {
		arrayLogger = l.loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	zeroLoggerEvent.Array("components", arrayLogger)
	zeroLoggerEvent.Int("entity_id", int(entity.EntityID()))
	return zeroLoggerEvent.Int("archetype_id", int(entity.Loc.ArchID)), nil
}

// LogComponents logs all component info related to the world
func (l *Logger) LogComponents(target Loggable, level zerolog.Level) {
	zeroLoggerEvent := l.WithLevel(level)
	zeroLoggerEvent = l.loadComponentsToEvent(zeroLoggerEvent, target)
	zeroLoggerEvent.Send()
}

// LogSystem logs all system info related to the world
func (l *Logger) LogSystem(target Loggable, level zerolog.Level) {
	zeroLoggerEvent := l.WithLevel(level)
	zeroLoggerEvent = l.loadSystemIntoEvent(zeroLoggerEvent, target)
	zeroLoggerEvent.Send()
}

// LogEntity logs entity info given an entityID
func (l *Logger) LogEntity(level zerolog.Level, entity entity.Entity, components []component.IComponentType) {
	zeroLoggerEvent := l.WithLevel(level)
	var err error = nil
	zeroLoggerEvent, err = l.loadEntityIntoEvent(zeroLoggerEvent, entity, components)
	if err != nil {
		l.Err(err).Msg(fmt.Sprintf("Error in Logger when retrieving entity with id %d", entity.EntityID()))
	} else {
		zeroLoggerEvent.Send()
	}
}

// LogWorld Logs everything about the world (components and Systems)
func (l *Logger) LogWorld(target Loggable, level zerolog.Level) {
	zeroLoggerEvent := l.WithLevel(level)
	zeroLoggerEvent = l.loadComponentsToEvent(zeroLoggerEvent, target)
	zeroLoggerEvent = l.loadSystemIntoEvent(zeroLoggerEvent, target)
	zeroLoggerEvent.Send()
}

// CreateSystemLogger creates a Sub Logger with the entry {"system" : systemName}
func (l *Logger) CreateSystemLogger(systemName string) Logger {
	zeroLogger := l.Logger.With().
		Str("system", systemName).Logger()
	return Logger{
		&zeroLogger,
	}
}

// CreateTraceLogger Creates a trace Logger. Using a single id you can use this Logger to follow and log a data path.
func (l *Logger) CreateTraceLogger(traceId string) zerolog.Logger {
	return l.Logger.With().
		Str("trace_id", traceId).
		Logger()
}
