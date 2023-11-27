package redis

type NonceStorage interface {
	GetNonce(key string) (uint64, error)
	SetNonce(key string, nonce uint64) error
}
