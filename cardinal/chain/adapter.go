package chain

//go:generate mockgen -source=adapter.go -package mocks -destination mocks/adapter.go
type Adapter interface {
	ReadAll() []byte
	Submit(bz []byte) error
}

type Config struct {
	Addr string
}

func NewAdapter(cfg Config) Adapter {
	return &adapterImpl{}
}

type adapterImpl struct {
	cfg Config
}

func (a adapterImpl) ReadAll() []byte {
	//TODO implement me
	panic("implement me")
}

func (a adapterImpl) Submit(bz []byte) error {
	//TODO implement me
	panic("implement me")
}
