package sequencer

type Option func(*Sequencer)

func WithSecretKey(key string) Option {
	return func(server *Sequencer) {
		server.key = key
	}
}
