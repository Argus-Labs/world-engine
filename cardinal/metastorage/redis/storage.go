package redis

import (
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

type MetaStorage struct {
	Namespace string
	Client    *redis.Client
	Log       zerolog.Logger
	NonceStorage
	SchemaStorage
}

type Options = redis.Options

func NewRedisMetaStorage(options Options, namespace string) MetaStorage {
	client := redis.NewClient(&options)
	return MetaStorage{
		Namespace:     namespace,
		Client:        client,
		Log:           zerolog.New(os.Stdout),
		NonceStorage:  NewNonceStorage(client),
		SchemaStorage: NewSchemaStorage(client),
	}
}

func (r *MetaStorage) Close() error {
	err := r.Client.Close()
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}
