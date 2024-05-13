package types

import "encoding/json"

// CqlData is the json result type that is returned to the user after executing cql.
type CqlData struct {
	ID   EntityID          `json:"id"`
	Data []json.RawMessage `json:"data" swaggertype:"object"`
}
