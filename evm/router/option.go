package router

type Option func(r *routerImpl)

// WithRouterKey sets the router key for the game shard <> base shard communications.
func WithRouterKey(key string) Option {
	return func(r *routerImpl) {
		r.key = key
	}
}
