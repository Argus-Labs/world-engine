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
	Nonce     NonceStorage
	Schema    SchemaStorage
}

type Options = redis.Options

func NewRedisStorage(options Options, namespace string) Storage {
	client := redis.NewClient(&options)
	return Storage{
		Namespace: namespace,
		Client:    client,
		Log:       zerolog.New(os.Stdout),
		Nonce:     NewNonceStorage(client),
		Schema:    NewSchemaStorage(client),
	}
}

func (r *Storage) Close() error {
	err := r.Client.Close()
	if err != nil {
		return eris.Wrap(err, "")
	}
	return nil
}
