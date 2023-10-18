package router

import "time"

type Option func(r *routerImpl)

// WithCredentials sets the SSH credentials for the gRPC server.
func WithCredentials(credPath string) Option {
	return func(r *routerImpl) {
		c, err := loadClientCredentials(credPath)
		if err != nil {
			panic(err)
		}
		r.creds = c
	}
}

func WithResultsKeepAlive(d time.Duration) Option {
	return func(r *routerImpl) {
		r.results.keepAlive = d
	}
}
