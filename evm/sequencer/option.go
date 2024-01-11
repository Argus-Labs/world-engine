package sequencer

type Option func(*Sequencer)

func WithCredentials(certPath, keyPath string) Option {
	return func(server *Sequencer) {
		creds, err := loadCredentials(certPath, keyPath)
		if err != nil {
			panic(err)
		}
		server.creds = creds
	}
}
