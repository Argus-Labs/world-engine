package shard

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
		a.creds = creds
	}
}
