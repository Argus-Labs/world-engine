package systemevent

import "github.com/goccy/go-json"

type PlayerDeath struct {
	Nickname string
}

func (PlayerDeath) Name() string {
	return "player-death"
}

// MarshalWire / UnmarshalWire satisfy the wire contract that every command, event, component, and system
// event implements. This package has no proto contract, so the codec is hand-written.
func (c PlayerDeath) MarshalWire() ([]byte, error) {
	return json.Marshal(c)
}

func (c PlayerDeath) UnmarshalWire(data []byte) (any, error) {
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return c, nil
}
