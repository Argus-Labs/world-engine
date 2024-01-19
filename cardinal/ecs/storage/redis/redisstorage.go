package redis

import (
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

type Storage struct {
	Namespace string
	Client    *redis.Client
	Log       zerolog.Logger
	RedisNonceStorage
	RedisSchemaStorage
}

type Options = redis.Options

func NewRedisStorage(options Options, namespace string) Storage {
	client := redis.NewClient(&options)
	return Storage{
		Namespace:          namespace,
		Client:             client,
		Log:                zerolog.New(os.Stdout),
		RedisNonceStorage:  NewRedisNonceStorage(client),
		RedisSchemaStorage: NewRedisSchemaStorage(client),
	}
}

func (r *Storage) Close() error {
	err := r.Client.Close()
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}
