package sequencer

type Option func(*Sequencer)

func WithRouterKey(key string) Option {
	return func(server *Sequencer) {
		server.routerKey = key
	}
}
