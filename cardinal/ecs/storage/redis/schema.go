package redis

import (
	"context"

	"github.com/rotisserie/eris"
)

type SchemaStorage interface {
	GetSchema(componentName string) ([]byte, error)
	SetSchema(componentName string, schemaData []byte) error
}

func (r *RedisStorage) GetSchema(componentName string) ([]byte, error) {
	ctx := context.Background()
	schemaBytes, err := r.Client.HGet(ctx, r.schemaStorageKey(), componentName).Bytes()
	err = eris.Wrap(err, "")
	if err != nil {
		return nil, err
	}
	return schemaBytes, nil
}

func (r *RedisStorage) SetSchema(componentName string, schemaData []byte) error {
	ctx := context.Background()
	return eris.Wrap(r.Client.HSet(ctx, r.schemaStorageKey(), componentName, schemaData).Err(), "")
}

var _ NonceStorage = &RedisStorage{}
