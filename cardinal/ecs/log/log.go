package log

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type Loggable interface {
	GetComponents() []component.ComponentMetadata
	GetSystemNames() []string
}

func loadComponentIntoArrayLogger(
	component component.ComponentMetadata,
	arrayLogger *zerolog.Array,
) *zerolog.Array {
	dictLogger := zerolog.Dict()
	dictLogger = dictLogger.Int("component_id", int(component.ID()))
	dictLogger = dictLogger.Str("component_name", component.Name())
	return arrayLogger.Dict(dictLogger)
}

func loadComponentsToEvent(zeroLoggerEvent *zerolog.Event, target Loggable) *zerolog.Event {
	zeroLoggerEvent.Int("total_components", len(target.GetComponents()))
	arrayLogger := zerolog.Arr()
	for _, _component := range target.GetComponents() {
		arrayLogger = loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	return zeroLoggerEvent.Array("components", arrayLogger)
}

func loadSystemIntoArrayLogger(name string, arrayLogger *zerolog.Array) *zerolog.Array {
	return arrayLogger.Str(name)
}

func loadSystemIntoEvent(zeroLoggerEvent *zerolog.Event, target Loggable) *zerolog.Event {
	zeroLoggerEvent.Int("total_systems", len(target.GetSystemNames()))
	arrayLogger := zerolog.Arr()
	for _, name := range target.GetSystemNames() {
		arrayLogger = loadSystemIntoArrayLogger(name, arrayLogger)
	}
	return zeroLoggerEvent.Array("systems", arrayLogger)
}

func loadEntityIntoEvent(
	zeroLoggerEvent *zerolog.Event, entityID entity.ID, archID archetype.ID,
	components []component.ComponentMetadata,
) *zerolog.Event {
	arrayLogger := zerolog.Arr()
	for _, _component := range components {
		arrayLogger = loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	zeroLoggerEvent.Array("components", arrayLogger)
	zeroLoggerEvent.Int("entity_id", int(entityID))
	return zeroLoggerEvent.Int("archetype_id", int(archID))
}

// Components logs all component info related to the world.
func Components(logger *zerolog.Logger, target Loggable, level zerolog.Level) {
	zeroLoggerEvent := logger.WithLevel(level)
	zeroLoggerEvent = loadComponentsToEvent(zeroLoggerEvent, target)
	zeroLoggerEvent.Send()
}

// System logs all system info related to the world.
func System(logger *zerolog.Logger, target Loggable, level zerolog.Level) {
	zeroLoggerEvent := logger.WithLevel(level)
	zeroLoggerEvent = loadSystemIntoEvent(zeroLoggerEvent, target)
	zeroLoggerEvent.Send()
}

// LogEntity logs entity info given an entityID.
func Entity(
	logger *zerolog.Logger,
	level zerolog.Level, entityID entity.ID, archID archetype.ID,
	components []component.ComponentMetadata,
) {
	zeroLoggerEvent := logger.WithLevel(level)
	loadEntityIntoEvent(zeroLoggerEvent, entityID, archID, components).Send()
}

// World Logs everything about the world (components and Systems).
func World(logger *zerolog.Logger, target Loggable, level zerolog.Level) {
	zeroLoggerEvent := logger.WithLevel(level)
	zeroLoggerEvent = loadComponentsToEvent(zeroLoggerEvent, target)
	zeroLoggerEvent = loadSystemIntoEvent(zeroLoggerEvent, target)
	zeroLoggerEvent.Send()
}

// CreateSystemLogger creates a Sub Logger with the entry {"system" : systemName}.
func CreateSystemLogger(logger *zerolog.Logger, systemName string) *zerolog.Logger {
	newLogger := logger.With().Str("system", systemName).Logger()
	return &newLogger
}

// CreateTraceLogger Creates a trace Logger. Using a single id you can use this Logger to follow and log a data path.
func CreateTraceLogger(logger *zerolog.Logger, traceID string) *zerolog.Logger {
	newLogger := logger.With().Str("trace_id", traceID).Logger()
	return &newLogger
}
