package types

import "encoding/json"

type EntityID uint64

type EntityData struct {
	ID         EntityID                   `json:"id"`
	Components map[string]json.RawMessage `json:"components" swaggertype:"object"`
}
