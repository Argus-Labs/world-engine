package sync

// ProtocolVersion 2 adds the per-reader Access list.
const ProtocolVersion = 2
const EventName = "world_sync"

// Frame is one reader's baseline, or the ordered change to what that reader may see.
// Components omitted from a Record are absent. Records omitted from a delta are unchanged.
// Records are shared state; a reader's own access travels in Access, so an access change
// never bumps a revision.
type Frame struct {
	Version     int
	Fingerprint string
	Stream      string
	Sequence    uint64
	Baseline    bool
	Records     []Record
	Removed     []ID
	Access      []Grant
}

type Record struct {
	ID         ID
	Revision   uint64
	Presence   uint64
	Components []Value
}

type Value struct {
	Name     string
	Presence uint64
	Data     []byte
}

func (Frame) Name() string { return EventName }

// Access is what one reader may do with one entity. State and events need at least View;
// a command at the entity needs Authority.
type Access uint8

const (
	None Access = iota
	View
	Authority
)

// Grant is a reader's access to one entity lifetime. A frame lists a grant when the reader
// first sees the lifetime and whenever its access changes while it stays visible.
type Grant struct {
	ID     ID
	Access Access
}

// Occurrence marks live feedback that must not replay with a recovery baseline.
// Spawn and state-restatement events must not embed this marker.
type Occurrence struct{}

func (Occurrence) SyncOccurrence() {}
