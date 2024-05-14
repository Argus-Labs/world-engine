package types

import "encoding/json"

type EntityID uint64

type EntityStateResponse []EntityStateElement

type EntityStateRequest struct{}

type EntityStateElement struct {
	ID   EntityID          `json:"id"`
	Data []json.RawMessage `json:"data" swaggertype:"object"`
}
