package shard

type Option func(*Server)

func WithCredentials(certPath, keyPath string) Option {
	return func(server *Server) {
		creds, err := loadCredentials(certPath, keyPath)
		if err != nil {
			panic(err)
		}
		server.creds = creds
	}
}
