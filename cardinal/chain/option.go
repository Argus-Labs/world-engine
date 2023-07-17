package chain

type Option func(adapter *adapterImpl)

func WithCredentials(credPath string) Option {
	return func(a *adapterImpl) {
		creds, err := loadClientCredentials(credPath)
		if err != nil {
			panic(err)
		}
		a.creds = creds
	}
}
