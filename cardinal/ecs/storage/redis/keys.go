package redis

/*
	NONCE STORAGE:      ADDRESS_TO_NONCE -> Nonce used for verifying signatures.
	Hash set of signature address to uint64 nonce
*/

func (r *Storage) nonceKey() string {
	return "ADDRESS_TO_NONCE"
}

func (r *Storage) schemaStorageKey() string {
	return "COMPONENT_NAME_TO_SCHEMA_DATA"
}
