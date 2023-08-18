package ecs

import (
	"github.com/rs/zerolog"
	"path/filepath"
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

func (wl *WorldLogger) Log(traceId string, logLevel zerolog.Level, message string) {
	wl.logger.WithLevel(logLevel).Str("trace_id", traceId).Msg(message)
}

func (wl *WorldLogger) LogInfo(traceId string, message string) {
	wl.Log(traceId, zerolog.InfoLevel, message)
}

func (wl *WorldLogger) LogDebug(traceId string, message string) {
	wl.Log(traceId, zerolog.DebugLevel, message)

}

func (wl *WorldLogger) LogError(traceId string, message string) {
	wl.Log(traceId, zerolog.ErrorLevel, message)
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
	functionName := filepath.Base(runtime.FuncForPC(reflect.ValueOf(*system).Pointer()).Name())
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

func (wl *WorldLogger) LogEntity(traceId string, logLevel zerolog.Level, entityId storage.EntityID, message string) error {
	zeroLoggerEvent := wl.logger.WithLevel(logLevel)
	var err error = nil
	zeroLoggerEvent = zeroLoggerEvent.Str("trace_id", traceId)
	zeroLoggerEvent, err = wl.loadEntityInfoIntoLogger(entityId, zeroLoggerEvent)
	if err != nil {
		return err
	}
	zeroLoggerEvent.Msg(message)
	return nil
}

func (wl *WorldLogger) LogWorldState(traceId string, logLevel zerolog.Level, message string) {
	zeroLoggerEvent := wl.logger.WithLevel(logLevel)
	zeroLoggerEvent.Str("trace_id", traceId)
	zeroLoggerEvent = wl.loadComponentInfoToLogger(zeroLoggerEvent)
	zeroLoggerEvent = wl.loadSystemInfoToLogger(zeroLoggerEvent)
	zeroLoggerEvent.Msg(message)
}
