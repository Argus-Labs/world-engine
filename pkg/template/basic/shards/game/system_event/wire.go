package systemevent

import (
	event "github.com/argus-labs/world-engine/pkg/template/basic/shards/game/gen/pkg/template/basic/shards/game/event"
	"google.golang.org/protobuf/proto"
)

// Hand-written, unlike its siblings: `world sdk generate` emits no proto message for system events, so
// there is no system_event/wire.gen.go for it to live in. PlayerDeath still has to satisfy
// schema.Serializable to be usable with WithSystemEventEmitter/Receiver, so it borrows the generated
// event.PlayerDeath message, which is structurally identical (one string field, Nickname).
//
// The borrowing is the wart: change event.PlayerDeath's shape and you silently change this system
// event's encoding. Delete this file once the generator emits system-event protos of its own.

func (c PlayerDeath) ToProto() *event.PlayerDeath {
	p := &event.PlayerDeath{}
	p.Nickname = c.Nickname
	return p
}

func (c PlayerDeath) FromProto(p *event.PlayerDeath) PlayerDeath {
	if p == nil {
		return c
	}
	c.Nickname = p.GetNickname()
	return c
}

func (c PlayerDeath) MarshalWire() ([]byte, error) {
	return proto.Marshal(c.ToProto())
}

func (c PlayerDeath) UnmarshalWire(data []byte) (any, error) {
	var p event.PlayerDeath
	if err := proto.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return c.FromProto(&p), nil
}
