package router

type Option func(r *routerImpl)

// WithSecretKey sets the secret key for the game shard <> base shard communications.
func WithSecretKey(key string) Option {
	return func(r *routerImpl) {
		r.key = key
	}
}
