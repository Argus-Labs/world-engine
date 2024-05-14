package types

import "encoding/json"

type EntityID uint64

type EntityStateElement struct {
	ID   EntityID          `json:"id"`
	Data []json.RawMessage `json:"data" swaggertype:"object"`
}
