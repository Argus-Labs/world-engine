package evm

type Option func(*srv)

func WithCredentials(certPath, keyPath string) Option {
	return func(s *srv) {
		creds, err := loadCredentials(certPath, keyPath)
		if err != nil {
			panic(err)
		}
		s.creds = creds
	}
}
