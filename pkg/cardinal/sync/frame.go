package sync

// ProtocolVersion 2 adds the per-reader Access list and the compact frame layout.
const ProtocolVersion = 2
const EventName = "world_sync"

// Frame is one reader's baseline, or the ordered change to what that reader may see. A frame and
// everything in it encodes as MessagePack arrays in field order, with the smallest integer
// encodings; a nil list and an empty list mean the same. Lifetimes a delta does not mention are
// unchanged. Records are shared state; a reader's own access travels in Access, so an access
// change never bumps a revision.
type Frame struct {
	_msgpack struct{} `msgpack:",as_array"` //nolint:unused // msgpack reads the tag: encode as an array

	Open     *Open    // Set exactly on a baseline, which starts a stream at sequence 1
	Stream   uint64   // Numbers the stream; a delta carrying another number is stale
	Sequence uint64   // 1 on the baseline, then +1 per frame
	Records  []Record // Whole entities: lifetimes the reader starts to hold
	Patches  []Patch  // Changes to lifetimes the reader already holds; never on a baseline
	Removed  []Ref    // Lifetimes the reader no longer holds; never on a baseline
	Access   []Grant
}

// Open is what a stream says once: the protocol and contract it speaks, and the session every
// Ref in the stream belongs to. Version stays the first element of the first element.
type Open struct {
	_msgpack struct{} `msgpack:",as_array"` //nolint:unused // msgpack reads the tag: encode as an array

	Version     int
	Fingerprint string
	Session     string
}

// Ref names one entity lifetime within the stream's session. With Open.Session it is an ID.
type Ref struct {
	_msgpack struct{} `msgpack:",as_array"` //nolint:unused // msgpack reads the tag: encode as an array

	Entity      uint32
	Incarnation uint64
}

// RefOf returns the Ref of an entity lifetime.
func RefOf(id ID) Ref { return Ref{Entity: id.Entity, Incarnation: id.Incarnation} }

// ID returns the lifetime a Ref names in session.
func (r Ref) ID(session string) ID {
	return ID{Session: session, Entity: r.Entity, Incarnation: r.Incarnation}
}

// Record is an entity's whole synced state. Components it omits are absent.
type Record struct {
	_msgpack struct{} `msgpack:",as_array"` //nolint:unused // msgpack reads the tag: encode as an array

	Ref        Ref
	Revision   uint64
	Presence   uint64
	Components []Value
}

// Patch moves an entity the reader holds to Revision. Components it omits are unchanged, and the
// entity's presence stays.
type Patch struct {
	_msgpack struct{} `msgpack:",as_array"` //nolint:unused // msgpack reads the tag: encode as an array

	Ref      Ref
	Revision uint64
	Changed  []Value  // Components added or changed, including a changed presence
	Dropped  []uint16 // Components removed, by contract index
}

// Value is one component. Component indexes the contract's components sorted by name in byte
// order, the order Contract.CanonicalJSON writes them. Data is the component's MessagePack map.
type Value struct {
	_msgpack struct{} `msgpack:",as_array"` //nolint:unused // msgpack reads the tag: encode as an array

	Component uint16
	Presence  uint64
	Data      []byte
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
	_msgpack struct{} `msgpack:",as_array"` //nolint:unused // msgpack reads the tag: encode as an array

	Ref    Ref
	Access Access
}

// Occurrence marks live feedback that must not replay with a recovery baseline.
// Spawn and state-restatement events must not embed this marker.
type Occurrence struct{}

func (Occurrence) SyncOccurrence() {}
