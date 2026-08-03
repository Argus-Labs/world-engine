package systemevent

type PlayerDeath struct {
	Nickname string
}

func (PlayerDeath) Name() string {
	return "player-death"
}

// MarshalWire / UnmarshalWire satisfy the wire contract that every command, event, component and system
// event implements. System events are consumed in-process, and `world sdk generate` emits no proto
// contract for this package, so the codec is hand-written here: the payload is a single string, so it is
// its own encoding. Once the generator covers system_event packages these methods move to a generated
// wire.gen.go.
func (c PlayerDeath) MarshalWire() ([]byte, error) {
	return []byte(c.Nickname), nil
}

func (PlayerDeath) UnmarshalWire(data []byte) (any, error) {
	return PlayerDeath{Nickname: string(data)}, nil
}
