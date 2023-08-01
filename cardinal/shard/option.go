package shard

import "google.golang.org/grpc"

type Option func(adapter *adapterImpl)

func WithCredentials(credPath string) Option {
	return func(a *adapterImpl) {
		if credPath == "" {
			panic("must provide client credential path")
		}
		creds, err := loadClientCredentials(credPath)
		if err != nil {
			panic(err)
		}
		a.grpcOpts = append(a.grpcOpts, grpc.WithTransportCredentials(creds))
	}
}
