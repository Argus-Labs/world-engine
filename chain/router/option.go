package router

import "google.golang.org/grpc"

type Option func(r *router)

func WithCredentials(credPath string) Option {
	return func(r *router) {
		c, err := loadClientCredentials(credPath)
		if err != nil {
			panic(err)
		}
		r.clientOpts = append(r.clientOpts, grpc.WithTransportCredentials(c))
	}
}
