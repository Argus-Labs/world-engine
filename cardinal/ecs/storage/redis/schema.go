package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
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
	err = eris.Wrap(err, "")
	if err != nil {
		return nil, err
	}
	return schemaBytes, nil
}

func (r *SchemaStorage) SetSchema(componentName string, schemaData []byte) error {
	ctx := context.Background()
	return eris.Wrap(r.Client.HSet(ctx, r.schemaStorageKey(), componentName, schemaData).Err(), "")
}
