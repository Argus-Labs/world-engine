package cardinal_test

import (
	"maps"
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

// threat holds every collection shape a component may use, edited every tick by threatSystem.
type threat struct {
	Table immutable.Map[cardinal.EntityID, int64]
	Log   immutable.Slice[int64]
	ByTag immutable.Map[string, immutable.Slice[uint8]]
}

func (threat) Name() string { return "threat" }

type threatRow struct {
	Threat threat
}

type hitCommand struct {
	Attacker cardinal.EntityID
	Damage   int64
	Tags     immutable.Map[string, uint8]
}

func (hitCommand) Name() string { return "hit" }

// threatSystem keeps two entities alive and folds every hit into each one's threat, through Map and
// Slice builders, single derivations and nested values.
type threatSystem struct{}

func (*threatSystem) Run(w *cardinal.World) {
	n := 0
	for range w.Contains[threatRow]().Iter() {
		n++
	}
	for ; n < 2; n++ {
		w.Create[threatRow]()
	}

	var hits []hitCommand
	for cmd := range w.Commands[hitCommand]() {
		hits = append(hits, cmd.Payload)
	}
	for entity := range w.Contains[threatRow]().Iter() {
		c := entity.Get[threat]()
		table, log := c.Table.Edit(), c.Log.Edit()
		for _, hit := range hits {
			v, _ := table.Get(hit.Attacker)
			table.Set(hit.Attacker, v+hit.Damage)
			log.Append(hit.Damage)
			for tag, count := range hit.Tags.All() {
				c.ByTag = c.ByTag.Update(tag, func(s immutable.Slice[uint8]) immutable.Slice[uint8] {
					s = s.Append(count)
					return s.Sub(max(0, s.Len()-4), s.Len())
				})
			}
		}
		if log.Len() > 16 {
			log.Delete(0, log.Len()-16)
		}
		c.Table, c.Log = table.Done(), log.Done()
		for c.Table.Len() > 8 {
			for k := range c.Table.Keys() {
				c.Table = c.Table.Without(k)
				break
			}
		}
		entity.Set(c)
	}
}

func registerThreat(w *cardinal.World) {
	w.RegisterComponent[threat]()
	w.RegisterCommand[hitCommand]()
	w.RegisterSystem(&threatSystem{})
}

// TestImmutableCollections_Deterministic edits Map and Slice fields every tick from commands whose
// Maps are built from Go maps, whose iteration order differs between the two runs. The world state
// must still match byte for byte after every step.
func TestImmutableCollections_Deterministic(t *testing.T) {
	t.Parallel()

	cardinal.RequireDeterministic(t, registerThreat, func(w *cardinal.TestWorld) {
		for step := range 60 {
			tags := map[string]uint8{}
			for i := range step % 7 {
				tags["tag"+strconv.Itoa((step+i)%11)] = uint8(i)
			}
			w.Command("p"+strconv.Itoa(step%3), hitCommand{
				Attacker: cardinal.EntityID(step % 13),
				Damage:   int64(step*7%23) - 5,
				Tags:     immutable.MapFrom(maps.All(tags)),
			})
			w.Tick()
		}
	})
}

// TestImmutableCollections_DST runs the same system under DST, which fills hitCommand's Map with
// random entries and snapshots, restores and restarts the world at random.
func TestImmutableCollections_DST(t *testing.T) { //nolint:paralleltest // RunDST sets environment variables.
	cardinal.RunDST(t, registerThreat, nil)
}
