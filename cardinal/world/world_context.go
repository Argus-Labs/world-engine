package world

import (
	"fmt"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/gamestate/search"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/tick"
	"pkg.world.dev/world-engine/cardinal/types"
)

var nonFatalError = []error{
	gamestate.ErrEntityDoesNotExist,
	gamestate.ErrComponentNotOnEntity,
	gamestate.ErrComponentAlreadyOnEntity,
	gamestate.ErrEntityMustHaveAtLeastOneComponent,
}

type worldContext struct {
	systemName string
	writer     *gamestate.EntityCommandBuffer
	reader     gamestate.Reader
	events     []map[string]any
	tick       *tick.Tick
	pm         *PersonaManager
}

type WorldContext interface {
	WorldContextReadOnly
	EmitEvent(event map[string]any)
	EmitStringEvent(eventMsg string)

	setSystemName(systemName string)
	stateWriter() (*gamestate.EntityCommandBuffer, error)
	getTick() *tick.Tick
}

type WorldContextReadOnly interface {
	Timestamp() int64
	CurrentTick() int64
	Logger() *zerolog.Logger
	Search(compFilter filter.ComponentFilter) *search.Search
	Namespace() string
	GetPersona(personaTag string) (*Persona, types.EntityID, error)

	stateReader() gamestate.Reader
	personaManager() *PersonaManager
	inSystem() bool
}

var _ WorldContext = (*worldContext)(nil)
var _ WorldContextReadOnly = (*worldContext)(nil)

func NewWorldContext(state *gamestate.State, pm *PersonaManager, tick *tick.Tick) WorldContext {
	return &worldContext{
		systemName: "",
		writer:     state.ECB(),
		reader:     state.ECB(),
		tick:       tick,
		pm:         pm,
	}
}

func NewWorldContextReadOnly(state *gamestate.State, pm *PersonaManager) WorldContextReadOnly {
	return &worldContext{
		systemName: "",
		writer:     nil,
		reader:     state.FinalizedState(),
		events:     make([]map[string]any, 0),
		tick:       nil,
		pm:         pm,
	}
}

func (ctx *worldContext) EmitEvent(event map[string]any) {
	ctx.tick.RecordEvent(ctx.systemName, event)
}

func (ctx *worldContext) EmitStringEvent(eventMsg string) {
	ctx.tick.RecordEvent(ctx.systemName, map[string]any{"message": eventMsg})
}

func (ctx *worldContext) Timestamp() int64 {
	return ctx.tick.Timestamp
}

func (ctx *worldContext) CurrentTick() int64 {
	return ctx.tick.ID
}

func (ctx *worldContext) Search(compFilter filter.ComponentFilter) *search.Search {
	return search.New(ctx.stateReader(), compFilter)
}

func (ctx *worldContext) Logger() *zerolog.Logger {
	if ctx.systemName == "" {
		return &log.Logger
	}
	sysLogger := log.Logger.With().Int64("tick", ctx.tick.ID).Str("system", ctx.systemName).Logger()
	return &sysLogger
}

func (ctx *worldContext) Namespace() string {
	return string(ctx.tick.Namespace)
}

func (ctx *worldContext) GetPersona(personaTag string) (*Persona, types.EntityID, error) {
	return ctx.pm.Get(ctx, personaTag)
}

func (ctx *worldContext) setSystemName(systemName string) {
	ctx.systemName = systemName
}

func (ctx *worldContext) stateWriter() (*gamestate.EntityCommandBuffer, error) {
	if ctx.writer == nil {
		return nil, eris.New("world context does not have a state writer")
	}
	return ctx.writer, nil
}

func (ctx *worldContext) stateReader() gamestate.Reader {
	return ctx.reader
}

func (ctx *worldContext) personaManager() *PersonaManager {
	return ctx.pm
}

func (ctx *worldContext) getTick() *tick.Tick {
	return ctx.tick
}

func (ctx *worldContext) inSystem() bool {
	return ctx.writer != nil
}

// -----------------------------------------------------------------------------
// Private methods
// -----------------------------------------------------------------------------

// panicOnFatalError is a helper function to panic on non-deterministic errors (i.e. Redis error).
func panicOnFatalError(wCtx WorldContextReadOnly, err error) {
	fmt.Println(err)
	fmt.Println(isFatalError(err))
	fmt.Println(wCtx.inSystem())
	if err != nil && isFatalError(err) && wCtx.inSystem() {
		log.Logger.Panic().Err(err).Msgf("fatal error: %v", eris.ToString(err, true))
		panic(err)
	}
}

func isFatalError(err error) bool {
	for _, e := range nonFatalError {
		if eris.Is(err, e) {
			return false
		}
	}
	return true
}
