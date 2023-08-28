package evm

import "google.golang.org/grpc"

type Option func(*msgServerImpl)

func WithCredentials(certPath, keyPath string) Option {
	return func(s *msgServerImpl) {
		if certPath == "" || keyPath == "" {
			panic("must provide both cert and key path")
		}
		creds, err := loadCredentials(certPath, keyPath)
		if err != nil {
			panic(err)
		}
		s.serverOpts = append(s.serverOpts, grpc.Creds(creds))
	}
}

func WithPort(port string) Option {
	return func(impl *msgServerImpl) {
		impl.port = port
	}
}
