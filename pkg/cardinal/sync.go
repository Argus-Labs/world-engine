package cardinal

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
)

// ExportSyncContract runs the application's ordinary type registrations without environment,
// listeners, storage, telemetry exporters, or simulation initialization.
func ExportSyncContract(register func(*World)) ([]byte, error) {
	if register == nil {
		return nil, fmt.Errorf("sync export requires type registration")
	}
	w := &World{world: ecs.NewWorld(), commands: command.NewManager(), events: event.NewManager(1)}
	w.service = newService(w, AuthModeDev, "")
	register(w)
	return w.SyncContract().CanonicalJSON()
}

// EnableSync supplies the authoritative gameplay-session audience. The predicate runs only
// on the tick, against current game state. No connection receives sync before it is admitted.
// Call before StartGame. RegisterComponent remains the only roster of game components.
func (w *World) EnableSync(admit func(playerID string) bool) {
	if w.started {
		panic(ErrWorldStarted)
	}
	if admit == nil {
		panic("sync requires a session audience")
	}
	w.ensureSync().admit = admit
}

// SyncContract exports registered, tagged component types without starting the world.
func (w *World) SyncContract() syncstate.Contract {
	contract := syncstate.Contract{Version: syncstate.ProtocolVersion, Components: []syncstate.Component{}}
	if w.sync != nil {
		for _, c := range w.sync.components {
			c.Fields = slices.Clone(c.Fields)
			contract.Components = append(contract.Components, c)
		}
	}
	slices.SortFunc(contract.Components, func(a, b syncstate.Component) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return contract
}

type syncWorld struct {
	admit       func(string) bool
	components  map[ecs.ComponentID]syncstate.Component
	marker      ecs.ComponentID
	dirty       map[EntityID]bool
	presence    map[EntityID]map[ecs.ComponentID]uint64
	records     map[EntityID]syncstate.Record
	sequence    uint64
	fingerprint string
}

type syncCommit struct {
	sequence uint64
	records  []syncstate.Record
	removed  []syncstate.ID
}

func (w *World) ensureSync() *syncWorld {
	if w.sync != nil {
		return w.sync
	}
	id, err := w.world.RegisterComponent[syncstate.Sync]()
	if err != nil {
		panic(err)
	}
	s := &syncWorld{marker: id, components: map[ecs.ComponentID]syncstate.Component{}, dirty: map[EntityID]bool{}, presence: map[EntityID]map[ecs.ComponentID]uint64{}, records: map[EntityID]syncstate.Record{}}
	w.sync = s
	w.world.Observe(s.observe)
	w.events.RegisterHandler(event.KindSync, func(_ context.Context, evt event.Event) error {
		return w.service.publishSyncCommit(evt.Payload.(syncCommit))
	})
	return s
}

func (s *syncWorld) observe(m ecs.Mutation) {
	switch m.Kind {
	case ecs.Reset:
		for id := range s.records {
			s.dirty[id] = true
		}
		clear(s.presence)
	case ecs.Created, ecs.Destroyed:
		s.dirty[m.Entity] = true
		delete(s.presence, m.Entity)
	case ecs.ComponentSet, ecs.ComponentAdded, ecs.ComponentRemoved:
		_, tracked := s.components[m.Component]
		if !tracked && m.Component != s.marker {
			return
		}
		s.dirty[m.Entity] = true
		if m.Kind == ecs.ComponentAdded || m.Kind == ecs.ComponentRemoved {
			if s.presence[m.Entity] == nil {
				s.presence[m.Entity] = map[ecs.ComponentID]uint64{}
			}
			s.presence[m.Entity][m.Component]++
		}
	}
}

func (w *World) registerSync(t reflect.Type, name string, cid ecs.ComponentID) {
	c, exposed, err := syncstate.Describe(t, name)
	if err != nil {
		panic(err)
	}
	if exposed {
		state := w.ensureSync()
		state.components[cid] = c
		state.fingerprint = ""
	}
}

// captureSync is an internal publication barrier before occurrences and at the end of a tick.
// It observes dirty entities only. Canonical payload bytes are reused for all recipients.
func (w *World) captureSync(final bool) {
	s := w.sync
	if s == nil || len(s.dirty) == 0 {
		return
	}
	ids := make([]EntityID, 0, len(s.dirty))
	for id := range s.dirty {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	names := make([]string, 0, len(s.components))
	for _, c := range s.components {
		names = append(names, c.Name)
	}
	slices.Sort(names)
	commit := syncCommit{}
	for _, id := range ids {
		old, known := s.records[id]
		entity := w.Entity(id)
		if !entity.Alive() || !entity.Has[syncstate.Sync]() {
			delete(s.dirty, id)
			if known {
				commit.removed = append(commit.removed, old.ID)
				delete(s.records, id)
			}
			continue
		}
		identity := entity.SyncID()
		if known && old.ID != identity {
			commit.removed = append(commit.removed, old.ID)
			known = false
			delete(s.records, id)
		}
		if !known && !final {
			continue
		}
		delete(s.dirty, id)
		data, err := w.world.ReadObserved(id, names)
		if err != nil {
			panic(fmt.Errorf("capture sync: %w", err))
		}
		record := syncstate.Record{ID: identity, Revision: 1, Presence: max(uint64(1), s.presence[id][s.marker]), Components: []syncstate.Value{}}
		for cid, c := range s.components {
			value, present := data[c.Name]
			if !present {
				continue
			}
			epoch := s.presence[id][cid]
			if epoch == 0 {
				epoch = 1
			}
			record.Components = append(record.Components, syncstate.Value{Name: c.Name, Presence: epoch, Data: value})
		}
		slices.SortFunc(record.Components, func(a, b syncstate.Value) int {
			if a.Name < b.Name {
				return -1
			}
			if a.Name > b.Name {
				return 1
			}
			return 0
		})
		if known {
			record.Revision = old.Revision
			if old.Presence == record.Presence && sameSyncValues(old.Components, record.Components) {
				continue
			}
			record.Revision++
		}
		s.records[id] = record
		commit.records = append(commit.records, record)
	}
	if len(commit.records) == 0 && len(commit.removed) == 0 {
		return
	}
	s.sequence++
	commit.sequence = s.sequence
	w.events.Enqueue(event.Event{Kind: event.KindSync, Payload: commit})
}

func sameSyncValues(a, b []syncstate.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Presence != b[i].Presence || !bytes.Equal(a[i].Data, b[i].Data) {
			return false
		}
	}
	return true
}

func (w *World) stampSync(payload any) {
	if w.sync == nil {
		return
	}
	if identified, ok := payload.(interface {
		SyncEntity() uint32
		SetSyncIdentity(syncstate.ID)
	}); ok {
		identified.SetSyncIdentity(w.Entity(EntityID(identified.SyncEntity())).SyncID())
	}
}
