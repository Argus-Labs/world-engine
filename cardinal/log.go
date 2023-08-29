package cardinal

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type Logger struct {
	impl *ecs.Logger
}

func (l *Logger) LogComponents(world *World, level zerolog.Level) {
	l.impl.LogComponents(world.impl, level)
}

// LogSystem logs all system info related to the world
func (l *Logger) LogSystem(world *World, level zerolog.Level) {
	l.impl.LogSystem(world.impl, level)
}

// LogEntity logs entity info given an entityID
func (l *Logger) LogEntity(world *World, level zerolog.Level, entityID EntityID) error {
	return l.impl.LogEntity(world.impl, level, entityID)
}

// LogWorld Logs everything about the world (components and Systems)
func (l *Logger) LogWorld(world *World, level zerolog.Level) {
	l.impl.LogWorld(world.impl, level)
}

// CreateSystemLogger creates a Sub logger with the entry {"system" : systemName}
func (l *Logger) CreateSystemLogger(systemName string) Logger {
	log := l.impl.CreateSystemLogger(systemName)
	return Logger{
		impl: &log,
	}
}

// CreateTraceLogger Creates a trace logger. Using a single id you can use this logger to follow and log a data path.
func (l *Logger) CreateTraceLogger(traceId string) zerolog.Logger {
	return l.impl.CreateTraceLogger(traceId)
}
