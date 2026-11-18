package ecs

import (
	"github.com/google/uuid"
)

// Mutation describes a committed ECS change. Observers run on the tick goroutine and must
// not read the world inside the callback; lifecycle operations may hold its lock. The world is
// read-only while they run, so an observer that writes to it panics.
type Mutation struct {
	Entity    EntityID
	Component ComponentID
	Kind      MutationKind
}

type MutationKind uint8

const (
	Created MutationKind = iota
	Destroyed
	ComponentSet
	ComponentAdded
	ComponentRemoved
	Reset
)

// Observe adds an observer of every committed change. Observers are told in the order they were
// added, and each one first sees every entity already alive as Created. The first call starts the
// session and gives those entities their lifetimes. Registration precedes world initialization.
func (w *World) Observe(observer func(Mutation)) {
	ws := w.state
	first := len(ws.observers) == 0
	if first {
		ws.session = uuid.NewString()
		ws.incarnations = map[EntityID]uint64{}
	}
	ws.observers = append(ws.observers, observer)
	for _, eid := range w.LiveEntityIDs() {
		if first {
			ws.born(eid)
			continue
		}
		ws.tell(ws.observers[len(ws.observers)-1:], Mutation{Entity: eid, Kind: Created})
	}
}

// Identity returns a process/session-scoped lifetime, never a persisted numeric ID by itself.
func (w *World) Identity(eid EntityID) (string, uint64) {
	if !w.Alive(eid) {
		return "", 0
	}
	return w.state.session, w.state.incarnations[eid]
}

// Session returns the session every lifetime Identity returns belongs to; "" before Observe.
func (w *World) Session() string { return w.state.session }

// ReadObserved encodes only the requested columns, once for this publication boundary.
// The caller owns all returned bytes. Call only on the tick goroutine.
func (w *World) ReadObserved(eid EntityID, names []string) (map[string][]byte, error) {
	aid, ok := w.state.entityArch.get(eid)
	if !ok {
		return nil, ErrEntityNotFound
	}
	arch := w.state.archetypes[aid]
	row, _ := arch.rows.get(eid)
	result := make(map[string][]byte, len(names))
	for _, name := range names {
		cid, err := w.state.components.getID(name)
		if err != nil {
			return nil, err
		}
		if !arch.components.Contains(cid) {
			continue
		}
		data, err := arch.columns[arch.components.CountTo(cid)].encode(row)
		if err != nil {
			return nil, err
		}
		result[name] = data
	}
	return result, nil
}

func (ws *worldState) notify(m Mutation) {
	if len(ws.observers) != 0 {
		ws.tell(ws.observers, m)
	}
}

// tell runs observers with the world read-only: a change is being committed, often under the
// world's lock, so a write from an observer would deadlock or corrupt it.
func (ws *worldState) tell(observers []func(Mutation), m Mutation) {
	previous := ws.readOnly
	ws.readOnly = "a mutation observer"
	defer func() { ws.readOnly = previous }()
	for _, observe := range observers {
		observe(m)
	}
}

func (ws *worldState) born(eid EntityID) {
	if len(ws.observers) == 0 {
		return
	}
	ws.nextIncarnation++
	if ws.nextIncarnation == 0 {
		panic("entity incarnation exhausted")
	}
	ws.incarnations[eid] = ws.nextIncarnation
	ws.notify(Mutation{Entity: eid, Kind: Created})
}
