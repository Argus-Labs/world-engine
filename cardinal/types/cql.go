package types

import "encoding/json"

type CqlData struct {
	ID   EntityID          `json:"id"`
	Data []json.RawMessage `json:"data" swaggertype:"object"`
}
