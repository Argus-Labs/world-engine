package types

import "encoding/json"

type DebugStateRequest struct{}

type DebugStateElement struct {
	ID         EntityID                   `json:"id"`
	Components map[string]json.RawMessage `json:"components" swaggertype:"object"`
}

type EntityStateResponse []DebugStateElement
