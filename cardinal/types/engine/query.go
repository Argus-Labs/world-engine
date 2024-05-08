package engine

type QueryHandler = func(name string, group string, bz []byte) ([]byte, error)
