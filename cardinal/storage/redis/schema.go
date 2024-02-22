package redis

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
)

var (
	ErrNoSchemaFound = errors.New("no schema found")
)

type SchemaStorage struct {
	Client *redis.Client
}

func NewSchemaStorage(client *redis.Client) SchemaStorage {
	return SchemaStorage{
		Client: client,
	}
}

func (r *SchemaStorage) GetSchema(componentName string) ([]byte, error) {
	ctx := context.Background()
	schemaBytes, err := r.Client.HGet(ctx, r.schemaStorageKey(), componentName).Bytes()
	if eris.Is(err, redis.Nil) {
		return nil, eris.Wrap(err, ErrNoSchemaFound.Error())
	} else if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return schemaBytes, nil
}

func (r *SchemaStorage) SetSchema(componentName string, schemaData []byte) error {
	ctx := context.Background()
	return eris.Wrap(r.Client.HSet(ctx, r.schemaStorageKey(), componentName, schemaData).Err(), "")
}
