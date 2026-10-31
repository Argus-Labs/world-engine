// Package sync declares the state an authoritative world exposes to its clients.
package sync

// State opts a registered component into sync. Unmarked components remain private.
// Embed it anonymously; it carries no runtime or MessagePack fields.
type State struct{}

// Sync opts an entity into sync. Embed it in the entity's archetype.
// Cardinal registers this built-in component when sync is enabled.
type Sync struct{}

func (Sync) Name() string { return "sync" }

// ID names one entity lifetime. Numeric entity IDs alone can be reused.
type ID struct {
	Session     string
	Entity      uint32
	Incarnation uint64
}

// Event carries the identity of a game occurrence or spawn. Its containing payload supplies
// SyncEntity() uint32. Cardinal stamps it when first emitted, before a numeric ID can be reused.
type Event struct {
	SyncSession     string
	SyncIncarnation uint64
}

func (e *Event) SetSyncIdentity(id ID) {
	if e.SyncSession == "" && e.SyncIncarnation == 0 {
		e.SyncSession = id.Session
		e.SyncIncarnation = id.Incarnation
	}
}

// For captures an identity for an event that will be emitted after destruction.
func For(id ID) Event { return Event{SyncSession: id.Session, SyncIncarnation: id.Incarnation} }
