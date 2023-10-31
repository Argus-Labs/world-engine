package cardinal

import (
	"fmt"

	"github.com/rs/zerolog"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
)

type Logger struct {
	impl *ecslog.Logger
}

func (l *Logger) LogComponents(world *World, level zerolog.Level) {
	l.impl.LogComponents(world.implWorld, level)
}

// LogSystem logs all system info related to the world.
func (l *Logger) LogSystem(world *World, level zerolog.Level) {
	l.impl.LogSystem(world.implWorld, level)
}

// LogEntity logs entity info given an entityID.
func (l *Logger) LogEntity(world *World, level zerolog.Level, entityID EntityID) {
	components, err := world.implWorld.StoreManager().GetComponentTypesForEntity(entityID)
	if err != nil {
		l.impl.Warn().Err(fmt.Errorf("failed to get components for entity %d: %w", entityID, err))
		return
	}
	archID, err := world.implWorld.StoreManager().GetArchIDForComponents(components)
	if err != nil {
		l.impl.Warn().Err(fmt.Errorf("failed to get archID for %d: %w", entityID, err))
		return
	}

	l.impl.LogEntity(level, entityID, archID, components)
}

// LogWorld Logs everything about the world (components and Systems).
func (l *Logger) LogWorld(world *World, level zerolog.Level) {
	l.impl.LogWorld(world.implWorld, level)
}

// CreateSystemLogger creates a Sub logger with the entry {"system" : systemName}.
func (l *Logger) CreateSystemLogger(systemName string) Logger {
	log := l.impl.CreateSystemLogger(systemName)
	return Logger{
		impl: &log,
	}
}

// CreateTraceLogger Creates a trace logger. Using a single id you can use this logger to follow and log a data path.
func (l *Logger) CreateTraceLogger(traceID string) zerolog.Logger {
	return l.impl.CreateTraceLogger(traceID)
}
