package ecs

import (
	"github.com/google/uuid"
)

// Mutation describes a committed ECS change. Observers run on the tick goroutine and must
// not read or mutate the world inside the callback; lifecycle operations may hold its lock.
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

// Observe enables change tracking for one consumer. Registration precedes world initialization.
func (w *World) Observe(observer func(Mutation)) {
	if w.state.observer != nil {
		panic("ECS already has a mutation observer")
	}
	w.state.observer = observer
	w.state.session = uuid.NewString()
	w.state.incarnations = map[EntityID]uint64{}
	for _, eid := range w.LiveEntityIDs() {
		w.state.born(eid)
	}
}

// Identity returns a process/session-scoped lifetime, never a persisted numeric ID by itself.
func (w *World) Identity(eid EntityID) (string, uint64) {
	if !w.Alive(eid) {
		return "", 0
	}
	return w.state.session, w.state.incarnations[eid]
}

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
	if ws.observer != nil {
		ws.observer(m)
	}
}
func (ws *worldState) born(eid EntityID) {
	if ws.observer == nil {
		return
	}
	ws.nextIncarnation++
	if ws.nextIncarnation == 0 {
		panic("entity incarnation exhausted")
	}
	ws.incarnations[eid] = ws.nextIncarnation
	ws.notify(Mutation{Entity: eid, Kind: Created})
}
