package adapter

import "github.com/rotisserie/eris"

type Option func(adapter *adapterImpl)

func WithCredentials(credPath string) Option {
	return func(a *adapterImpl) {
		if credPath == "" {
			panic("must provide client credential path")
		}
		creds, err := loadClientCredentials(credPath)
		if err != nil {
			panic(eris.ToString(err, true))
		}
		a.creds = creds
	}
}
