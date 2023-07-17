package evm

type Option func(*srv)

func WithCredentials(certPath, keyPath string) Option {
	return func(s *srv) {
		if certPath == "" || keyPath == "" {
			panic("must provide both cert and key path")
		}
		creds, err := loadCredentials(certPath, keyPath)
		if err != nil {
			panic(err)
		}
		s.creds = creds
	}
}
