package sync

const ProtocolVersion = 1
const EventName = "world_sync"

// Frame is a complete baseline or an ordered set of complete entity records.
// Components omitted from a Record are absent. Records omitted from a delta are unchanged.
type Frame struct {
	Version     int
	Fingerprint string
	Stream      string
	Sequence    uint64
	Baseline    bool
	Records     []Record
	Removed     []ID
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

// Occurrence marks live feedback that must not replay with a recovery baseline.
// Spawn and state-restatement events must not embed this marker.
type Occurrence struct{}

func (Occurrence) SyncOccurrence() {}
