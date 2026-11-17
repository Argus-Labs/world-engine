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
	Table immutable.Map[string, int64] // keyed by attacking player, an ID the game owns
	Log   immutable.Slice[int64]
	ByTag immutable.Map[string, immutable.Slice[uint8]]
}

func (threat) Name() string { return "threat" }

type threatRow struct {
	Threat threat
}

type hitCommand struct {
	Attacker string
	Damage   int64
	Tags     immutable.Map[string, uint8]
}

func (hitCommand) Name() string { return "hit" }

// threatSystem keeps two entities alive and folds every hit into each one's threat, through in-place
// Map and Slice edits and Slices nested in Map values.
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
		for _, hit := range hits {
			v, _ := c.Table.Get(hit.Attacker)
			c.Table.Set(hit.Attacker, v+hit.Damage)
			c.Log.Append(hit.Damage)
			for tag, count := range hit.Tags.All() {
				s, _ := c.ByTag.Get(tag)
				s.Append(count)
				s.Delete(0, max(0, s.Len()-4))
				c.ByTag.Set(tag, s)
			}
		}
		if c.Log.Len() > 16 {
			c.Log.Delete(0, c.Log.Len()-16)
		}
		for c.Table.Len() > 8 {
			for k := range c.Table.Keys() {
				c.Table.Delete(k)
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
				Attacker: "player-" + strconv.Itoa(step%13),
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
