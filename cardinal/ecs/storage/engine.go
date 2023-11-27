package storage

type NonceStorage interface {
	GetNonce(key string) (uint64, error)
	SetNonce(key string, nonce uint64) error
}

type SchemaStorage interface {
	GetSchema(componentName string) ([]byte, error)
	SetSchema(componentName string, schemaData []byte) error
}

type SchemaAndNonceStorage interface {
	NonceStorage
	SchemaStorage
}
