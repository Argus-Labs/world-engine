package cardinal

import "pkg.world.dev/world-engine/cardinal/ecs"

// AnyQueryType is implemented by the return value of NewQueryType and is used in RegisterQueries; any
// query operation creates by NewQueryType can be registered with a World object via RegisterQueries.
type AnyQueryType interface {
	Convert() ecs.IQuery
}

// QueryType represents a query operation on a world object. The state of the world object must not be
// changed during the query operation.
type QueryType[Request, Reply any] struct {
	impl *ecs.QueryType[Request, Reply]
}

// NewQueryType creates a new instance of a QueryType. The World state must not be changed
// in the given handler function.
func NewQueryType[Request any, Reply any](
	name string,
	handler func(WorldContext, Request) (Reply, error),
) *QueryType[Request, Reply] {
	return &QueryType[Request, Reply]{
		impl: ecs.NewQueryType[Request, Reply](name, func(wCtx ecs.WorldContext, req Request) (Reply, error) {
			return handler(wCtx, req)
		}),
	}
}

// NewQueryTypeWithEVMSupport creates a new instance of a QueryType with EVM support, allowing this query to be called from
// the EVM base shard. The World state must not be changed in the given handler function.
func NewQueryTypeWithEVMSupport[Request, Reply any](name string, handler func(WorldContext, Request) (Reply, error)) *QueryType[Request, Reply] {
	return &QueryType[Request, Reply]{
		impl: ecs.NewQueryType[Request, Reply](name, func(wCtx ecs.WorldContext, req Request) (Reply, error) {
			return handler(wCtx, req)
		}, ecs.WithQueryEVMSupport[Request, Reply]),
	}
}

// Convert implements the AnyQueryType interface which allows a QueryType to be registered
// with a World via RegisterQueries.
func (r *QueryType[Request, Reply]) Convert() ecs.IQuery {
	return r.impl
}
