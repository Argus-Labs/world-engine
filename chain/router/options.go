package router

type Option func(*router)

func WithNamespaces(m NamespaceClients) Option {
	return func(r *router) {
		r.namespaces = m
	}
}
