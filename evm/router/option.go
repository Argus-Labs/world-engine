package router

type Option func(r *routerImpl)

// WithRouterKey sets the router routerKey for the game shard <> base shard communications.
func WithRouterKey(key string) Option {
	return func(r *routerImpl) {
		r.routerKey = key
	}
}
