package redis

type EngineStorage interface {
	NonceStorage
	SchemaStorage
}
