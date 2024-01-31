package router

type Option func(r *Router)

func WithPort(port string) Option {
	return func(r *Router) {
		r.port = port
	}
}
