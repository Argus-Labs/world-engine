package cardinal

import (
	"reflect"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/types"
)

var NonFatalError = []error{
	ErrEntityDoesNotExist,
	ErrComponentNotOnEntity,
	ErrComponentAlreadyOnEntity,
	ErrEntityMustHaveAtLeastOneComponent,
}

// separateOptions separates the given options into ecs options, server options, and cardinal (this package) options.
// The different options are all grouped together to simplify the end user's experience, but under the hood different
// options are meant for different sub-systems.
func separateOptions(opts []WorldOption) (
	serverOptions []server.Option,
	cardinalOptions []Option,
) {
	for _, opt := range opts {
		if opt.serverOption != nil {
			serverOptions = append(serverOptions, opt.serverOption)
		}
		if opt.cardinalOption != nil {
			cardinalOptions = append(cardinalOptions, opt.cardinalOption)
		}
	}
	return serverOptions, cardinalOptions
}

// panicOnFatalError is a helper function to panic on non-deterministic errors (i.e. Redis error).
func panicOnFatalError(wCtx Context, err error) {
	if err != nil && !wCtx.isReadOnly() && isFatalError(err) {
		wCtx.Logger().Panic().Err(err).Msgf("fatal error: %v", eris.ToString(err, true))
		panic(err)
	}
}

func isFatalError(err error) bool {
	for _, e := range NonFatalError {
		if eris.Is(err, e) {
			return false
		}
	}
	return true
}

func GetMessage[In any, Out any](wCtx Context) (*MessageType[In, Out], error) {
	var msg MessageType[In, Out]
	msgType := reflect.TypeOf(msg)
	tempRes, ok := wCtx.getMessageByType(msgType)
	if !ok {
		return nil, eris.Errorf("Could not find %q, Message may not be registered.", msg.Name())
	}
	var _ types.Message = &msg
	res, ok := tempRes.(*MessageType[In, Out])
	if !ok {
		return &msg, eris.New("wrong type")
	}
	return res, nil
}

func GetTransactionReceiptsForTick(wCtx Context, tick uint64) ([]receipt.Receipt, error) {
	ctx, ok := wCtx.(*worldContext)
	if !ok {
		return nil, eris.New("error in test type assertion.")
	}
	return ctx.world.GetTransactionReceiptsForTick(tick)
}

func GetStoreManagerFromContext(wCtx Context) gamestate.Manager {
	return wCtx.storeManager()
}

func GetComponentByNameFromContext(wCtx Context, name string) (types.ComponentMetadata, error) {
	return wCtx.getComponentByName(name)
}

func HandleQuery(wCtx Context, query Query, a any) (any, error) {
	return query.handleQuery(wCtx, a)
}
