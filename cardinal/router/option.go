package router

type Option func(r *routerImpl)

func WithPort(port string) Option {
	return func(r *routerImpl) {
		r.port = port
	}
}
