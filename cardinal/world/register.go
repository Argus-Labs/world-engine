package world

import (
	"reflect"

	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

type Plugin interface {
	Register(w *World) error
}

func RegisterPlugin(w *World, plugin Plugin) {
	if err := plugin.Register(w); err != nil {
		log.Fatal().Err(err).Msgf("failed to register plugin: %v", err)
	}
}

func RegisterSystems(w *World, sys ...System) error {
	return w.RegisterSystems(false, sys...)
}

func RegisterInitSystems(w *World, sys ...System) error {
	return w.RegisterSystems(true, sys...)
}

func RegisterComponent[T types.Component](w *World) error {
	compMetadata, err := gamestate.NewComponentMetadata[T]()
	if err != nil {
		return err
	}
	return w.State().RegisterComponent(compMetadata)
}

// RegisterMessage registers a message to the world. Cardinal will automatically set up HTTP routes that map to each
// registered message. Message URLs are take the form of "group.name". A default group, "game", is used
// unless the WithCustomGroup option is used. Example: game.throw-rock
func RegisterMessage[Msg message.Message](w *World, opts ...message.Option[Msg]) error {
	msgType := message.NewMessageType[Msg](opts...)
	return w.RegisterMessage(msgType, reflect.TypeOf(msgType))
}

func RegisterQuery[Request any, Reply any](
	w *World,
	name string,
	handler func(wCtx WorldContextReadOnly, req *Request) (*Reply, error),
	opts ...QueryOption[Request, Reply],
) (err error) {
	q, err := NewQueryType[Request, Reply](name, handler, opts...)
	if err != nil {
		return err
	}
	return w.RegisterQuery(q)
}
