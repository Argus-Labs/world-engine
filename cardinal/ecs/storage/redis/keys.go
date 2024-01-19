package redis

import "fmt"

/*
	NONCE STORAGE:      ADDRESS_TO_NONCE -> Nonce used for verifying signatures.
	Hash set of signature address to uint64 nonce
*/

func (r *RedisNonceStorage) nonceSetKey(str string) string {
	return fmt.Sprintf("USED_NONCES_%s", str)
}

func (r *RedisSchemaStorage) schemaStorageKey() string {
	return "COMPONENT_NAME_TO_SCHEMA_DATA"
}
