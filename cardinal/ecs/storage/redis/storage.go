package redis

import (
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

type EngineStorage interface {
	NonceStorage
	SchemaStorage
}

type RedisStorage struct {
	Namespace string
	Client    *redis.Client
	Log       zerolog.Logger
}

type Options = redis.Options

func NewRedisStorage(options Options, namespace string) RedisStorage {
	return RedisStorage{
		Namespace: namespace,
		Client:    redis.NewClient(&options),
		Log:       zerolog.New(os.Stdout),
	}
}

var _ SchemaStorage = &RedisStorage{}

func (r *RedisStorage) Close() error {
	err := r.Client.Close()
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}
