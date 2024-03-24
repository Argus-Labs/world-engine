package redis

import (
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Storage struct {
	Namespace string
	Client    *redis.Client
	Log       zerolog.Logger
	NonceStorage
	SchemaStorage
}

type Options = redis.Options

func NewRedisStorage(options Options, namespace string) Storage {
	client := redis.NewClient(&options)
	return Storage{
		Namespace:     namespace,
		Client:        client,
		Log:           zerolog.New(os.Stdout),
		NonceStorage:  NewNonceStorage(client),
		SchemaStorage: NewSchemaStorage(client),
	}
}

func (r *Storage) Close() error {
	log.Info().Msg("Closing storage connection.")
	if err := r.Client.Close(); err != nil {
		return eris.Wrap(err, "")
	}
	log.Info().Msg("Successfully closed storage connection.")
	return nil
}
