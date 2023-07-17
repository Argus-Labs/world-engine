package router

type Option func(r *router)

func WithCredentials(credPath string) Option {
	return func(r *router) {
		c, err := loadClientCredentials(credPath)
		if err != nil {
			panic(err)
		}
		r.credential = c
	}
}
