package world_logger

import (
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"io"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"reflect"
	"runtime"
)

type IWorldLogger interface {
	LogWorldState(id string)
	LogInfo(id string, message string)
	LogDebug(id string, message string)
	LogError(id string, message string)
}

type WorldLogger struct {
	world  *ecs.World
	logger zerolog.Logger
}

func loadComponentIntoArrayLogger(component component.IComponentType, arrayLogger *zerolog.Array) *zerolog.Array {
	dictLogger := zerolog.Dict()
	dictLogger = dictLogger.Int("component_id", int(component.ID()))
	dictLogger = dictLogger.Str("component_name", component.Name())
	arrayLogger = arrayLogger.Dict(dictLogger)

	return arrayLogger
}

func (wl *WorldLogger) loadComponentInfoToLogger(zeroLoggerEvent *zerolog.Event) *zerolog.Event {
	zeroLoggerEvent = zeroLoggerEvent.Int("total_components", len(wl.world.ListRegisteredComponents()))
	arrayLogger := zerolog.Arr()
	for _, _component := range wl.world.ListRegisteredComponents() {
		arrayLogger = loadComponentIntoArrayLogger(_component, arrayLogger)
	}
	zeroLoggerEvent.Array("components", arrayLogger)
	return zeroLoggerEvent
}

func loadSystemIntoArrayLogger(system *ecs.System, arrayLogger *zerolog.Array) *zerolog.Array {
	functionName := runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name()
	return arrayLogger.Str(functionName)
}

func (wl *WorldLogger) loadSystemInfoToLogger(zeroLoggerEvent *zerolog.Event) *zerolog.Event {
	zeroLoggerEvent = zeroLoggerEvent.Int("total_systems", len(wl.world.ListSystems()))
	arrayLogger := zerolog.Arr()
	for _, system := range wl.world.ListSystems() {
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

func (wl *WorldLogger) LogWorldState(message string) {
	loggerEvent := wl.logger.Info()
	loggerEvent.Str("id", uuid.NewString())
	loggerEvent = wl.loadComponentInfoToLogger(loggerEvent)
	loggerEvent = wl.loadSystemInfoToLogger(loggerEvent)
	loggerEvent.Msg(message)
}

func NewWorldLogger(writer io.Writer, world *ecs.World) WorldLogger {
	if writer != nil {
		return WorldLogger{
			logger: zerolog.New(writer),
			world:  world,
		}
	} else {
		return WorldLogger{
			logger: log.Logger,
			world:  world,
		}
	}
}

func (wl *WorldLogger) LogInfo(id string, message string) {
	wl.logger.Info().Str("id", id).Msg(message)
}

func (wl *WorldLogger) LogDebug(id string, message string) {
	wl.logger.Debug().Str("id", id).Msg(message)
}

func (wl *WorldLogger) LogError(id string, message string) {
	wl.logger.Error().Str("id", id).Msg(message)
}
