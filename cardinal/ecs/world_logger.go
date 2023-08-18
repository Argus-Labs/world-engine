package ecs

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"reflect"
	"runtime"
)

type WorldLogger struct {
	world  *World
	logger *zerolog.Logger
}

func NewWorldLogger(logger *zerolog.Logger, world *World) WorldLogger {
	return WorldLogger{
		logger: logger,
		world:  world,
	}
}

func (wl *WorldLogger) LogInfo(trace_id string, message string) {
	wl.logger.Info().Str("trace_id", trace_id).Msg(message)
}

func (wl *WorldLogger) LogDebug(trace_id string, message string) {
	wl.logger.Debug().Str("trace_id", trace_id).Msg(message)
}

func (wl *WorldLogger) LogError(trace_id string, message string) {
	wl.logger.Error().Str("trace_id", trace_id).Msg(message)
}

func loadComponentIntoArrayLogger(component component.IComponentType, arrayLogger *zerolog.Array) *zerolog.Array {
	dictLogger := zerolog.Dict()
	dictLogger = dictLogger.Int("component_id", int(component.ID()))
	dictLogger = dictLogger.Str("component_name", component.Name())
	arrayLogger = arrayLogger.Dict(dictLogger)

	return arrayLogger
}

func (wl *WorldLogger) loadComponentInfoToLogger(zeroLoggerEvent *zerolog.Event) *zerolog.Event {
	zeroLoggerEvent = zeroLoggerEvent.Int("total_components", len(wl.world.registeredComponents))
	arrayLogger := zerolog.Arr()
	for _, _component := range wl.world.registeredComponents {
		arrayLogger = loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	zeroLoggerEvent.Array("components", arrayLogger)
	return zeroLoggerEvent
}

func loadSystemIntoArrayLogger(system *System, arrayLogger *zerolog.Array) *zerolog.Array {
	functionName := runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name()
	return arrayLogger.Str(functionName)
}

func (wl *WorldLogger) loadSystemInfoToLogger(zeroLoggerEvent *zerolog.Event) *zerolog.Event {
	zeroLoggerEvent = zeroLoggerEvent.Int("total_systems", len(wl.world.systems))
	arrayLogger := zerolog.Arr()
	for _, system := range wl.world.systems {
		arrayLogger = loadSystemIntoArrayLogger(&system, arrayLogger)
	}
	zeroLoggerEvent.Array("systems", arrayLogger)
	return zeroLoggerEvent
}

func (wl *WorldLogger) loadEntityInfoIntoLogger(entityID storage.EntityID, zeroLoggerEvent *zerolog.Event) (*zerolog.Event, error) {
	entity, err := wl.world.Entity(entityID)
	if err != nil {
		return nil, err
	}

	archetype := entity.Archetype(wl.world)
	arrayLogger := zerolog.Arr()
	for _, _component := range archetype.Layout().Components() {
		arrayLogger = loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	zeroLoggerEvent.Array("components", arrayLogger)
	zeroLoggerEvent.Int("entity_id", int(entityID))
	zeroLoggerEvent.Int("archetype_id", int(entity.Loc.ArchID))
	return zeroLoggerEvent, nil
}

func (wl *WorldLogger) LogDebugEntity(entityId storage.EntityID, message string) error {
	zeroLogger := wl.logger.Debug()
	var err error = nil
	zeroLogger, err = wl.loadEntityInfoIntoLogger(entityId, zeroLogger)
	if err != nil {
		return err
	}
	zeroLogger.Msg(message)
	return nil
}

func (wl *WorldLogger) LogWorldState(trace_id string, message string) {
	loggerEvent := wl.logger.Info()
	loggerEvent.Str("trace_id", trace_id)
	loggerEvent = wl.loadComponentInfoToLogger(loggerEvent)
	loggerEvent = wl.loadSystemInfoToLogger(loggerEvent)
	loggerEvent.Msg(message)
}
