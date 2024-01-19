package storage

type NonceStorage interface {
	UseNonce(signerAddress string, nonce uint64) error
}

type SchemaStore interface {
	GetSchema(componentName string) ([]byte, error)
	SetSchema(componentName string, schemaData []byte) error
}

type Storage interface {
	NonceStorage
	SchemaStore
	Close() error
}
