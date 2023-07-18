package shard

import "google.golang.org/grpc"

type Option func(*Server)

func WithCredentials(certPath, keyPath string) Option {
	return func(server *Server) {
		creds, err := loadCredentials(certPath, keyPath)
		if err != nil {
			panic(err)
		}
		server.serverOpts = append(server.serverOpts, grpc.Creds(creds))
	}
}
