package router

type Option func(r *router)

// WithCredentials sets the SSH credentials for the gRPC server.
func WithCredentials(credPath string) Option {
	return func(r *router) {
		c, err := loadClientCredentials(credPath)
		if err != nil {
			panic(err)
		}
		r.creds = c
	}
}
