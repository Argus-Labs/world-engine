package cardinal

import (
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/immutable"
)

// These tests run a small game whose systems use Slice and Map the way game code does: inventory
// stacks, a capped log, cooldowns, quest steps, a skill tree and a party that is emptied and refilled,
// fed by commands, events and system events that carry collections. A model of the same game on plain
// Go slices and maps runs beside it, and every tick the world must hold what the model holds, send
// the events the model expects, and survive a snapshot restore without drifting from the world that
// never stopped. Two peek systems edit everything they read without calling Set, before and after
// the game's systems, so any edit that leaks into the world also shows up as a mismatch.

const (
	ucPlayers = 3
	ucLogCap  = 6
	ucFromCap = 3
	ucRankCap = 4
	ucRoleCap = 3
	ucQuestAt = 5    // steps that finish a quest
	ucReward  = 900  // item granted for a finished quest
	ucNoItem  = 0xFF // item a peek or a late edit adds, which must never reach the world
)

// -------------------------------------------------------------------------------------------------
// Components
// -------------------------------------------------------------------------------------------------

type ucPlayer struct{ Who uint8 }

func (ucPlayer) Name() string { return "uc_player" }

// ucItemID is a named key type, which a Map stores and restores like its underlying type.
type ucItemID uint16

type ucStack struct {
	Count int32
	From  immutable.Slice[string] // the last ucFromCap sources
}

type ucInventory struct {
	Items immutable.Map[ucItemID, ucStack]
}

func (ucInventory) Name() string { return "uc_inventory" }

type ucAction struct {
	Tick uint64
	What string
}

type ucLog struct{ Entries immutable.Slice[ucAction] }

func (ucLog) Name() string { return "uc_log" }

type ucCooldowns struct{ ReadyAt immutable.Map[string, uint64] }

func (ucCooldowns) Name() string { return "uc_cooldowns" }

type ucQuests struct {
	Steps immutable.Map[string, immutable.Slice[uint8]]
}

func (ucQuests) Name() string { return "uc_quests" }

type ucSkills struct {
	Tree immutable.Map[string, immutable.Map[string, immutable.Slice[int32]]]
}

func (ucSkills) Name() string { return "uc_skills" }

type ucMember struct {
	Name  string
	Roles immutable.Slice[string] // the last ucRoleCap roles taken
}

type ucParty struct{ Members immutable.Slice[ucMember] }

func (ucParty) Name() string { return "uc_party" }

type ucRow struct {
	Player    ucPlayer
	Inventory ucInventory
	Log       ucLog
	Cooldowns ucCooldowns
	Quests    ucQuests
	Skills    ucSkills
	Party     ucParty
}

// -------------------------------------------------------------------------------------------------
// Commands, events and system events
// -------------------------------------------------------------------------------------------------

type ucPickup struct {
	Who   uint8
	From  string
	Items immutable.Map[ucItemID, int32] // a count of zero or less removes the stack
}

func (ucPickup) Name() string { return "uc_pickup" }

type ucCast struct {
	Who      uint8
	Ability  string
	Cooldown uint8
}

func (ucCast) Name() string { return "uc_cast" }

type ucProgress struct {
	Who   uint8
	Quest string
	Steps immutable.Slice[uint8]
}

func (ucProgress) Name() string { return "uc_progress" }

type ucLearn struct {
	Who    uint8
	Branch string
	Skill  string
	Ranks  immutable.Slice[int32]
}

func (ucLearn) Name() string { return "uc_learn" }

type ucJoin struct {
	Who    uint8
	Member string
	Role   string
}

func (ucJoin) Name() string { return "uc_join" }

type ucInventoryChanged struct {
	Who   uint8
	Items immutable.Map[ucItemID, ucStack]
}

func (ucInventoryChanged) Name() string { return "uc_inventory_changed" }

type ucCastResult struct {
	Ability string
	ReadyAt immutable.Map[string, uint64]
}

func (ucCastResult) Name() string { return "uc_cast_result" }

type ucQuestDone struct {
	Who   uint8
	Quest string
	Steps immutable.Slice[uint8]
}

func (ucQuestDone) Name() string { return "uc_quest_done" }

// ucQuestFinished is the system event the quest system emits for the reward and audit systems.
type ucQuestFinished struct {
	Who   uint8
	Quest string
	Steps immutable.Slice[uint8]
}

func (ucQuestFinished) Name() string { return "uc_quest_finished" }

// -------------------------------------------------------------------------------------------------
// Systems
// -------------------------------------------------------------------------------------------------

func ucRegister(w *World) {
	w.RegisterComponent[ucPlayer]()
	w.RegisterComponent[ucInventory]()
	w.RegisterComponent[ucLog]()
	w.RegisterComponent[ucCooldowns]()
	w.RegisterComponent[ucQuests]()
	w.RegisterComponent[ucSkills]()
	w.RegisterComponent[ucParty]()
	w.RegisterCommand[ucPickup]()
	w.RegisterCommand[ucCast]()
	w.RegisterCommand[ucProgress]()
	w.RegisterCommand[ucLearn]()
	w.RegisterCommand[ucJoin]()
	w.RegisterEvent[ucInventoryChanged]()
	w.RegisterEvent[ucCastResult]()
	w.RegisterEvent[ucQuestDone]()
	w.RegisterSystemEvent[ucQuestFinished]()

	w.RegisterSystem(&ucSpawn{}, WithHook(Init))
	w.RegisterSystem(&ucPeekBefore{}, WithHook(PreUpdate))
	w.RegisterSystem(&ucInventorySystem{})
	w.RegisterSystem(&ucAbilitySystem{})
	w.RegisterSystem(&ucQuestSystem{})
	w.RegisterSystem(&ucRewardSystem{})
	w.RegisterSystem(&ucAuditSystem{})
	w.RegisterSystem(&ucSkillSystem{})
	w.RegisterSystem(&ucPartySystem{})
	w.RegisterSystem(&ucPickupLogSystem{})
	w.RegisterSystem(&ucPeekAfter{}, WithHook(PostUpdate))
}

func ucFind(w *World, who uint8) Entity {
	for e := range w.Contains[ucRow]().Iter() {
		if e.Get[ucPlayer]().Who == who%ucPlayers {
			return e
		}
	}

	panic(fmt.Sprintf("player %d was not spawned", who%ucPlayers))
}

func ucRecipient(who uint8) string { return "p" + strconv.Itoa(int(who%ucPlayers)) }

func ucAppendLog(l *ucLog, tick uint64, what string) {
	l.Entries.Append(ucAction{Tick: tick, What: what})

	if n := l.Entries.Len(); n > ucLogCap {
		l.Entries.Delete(0, n-ucLogCap)
	}
}

func ucAddToStack(s ucStack, n int32, from string) ucStack {
	s.Count += n
	s.From.Append(from)

	if k := s.From.Len(); k > ucFromCap {
		s.From.Delete(0, k-ucFromCap)
	}

	return s
}

type ucSpawn struct{}

func (*ucSpawn) Run(w *World) {
	for who := range uint8(ucPlayers) {
		w.Create[ucRow]().Set(ucPlayer{Who: who})
	}
}

// ucInventorySystem stacks picked-up items, removes a stack whose count drops to zero, and tells
// clients which inventories changed.
type ucInventorySystem struct{}

func (*ucInventorySystem) Run(w *World) {
	var changed [ucPlayers]bool

	for cmd := range w.Commands[ucPickup]() {
		e := ucFind(w, cmd.Payload.Who)

		inv := e.Get[ucInventory]()
		for id, n := range cmd.Payload.Items.All() {
			s, _ := inv.Items.Get(id)
			if s.Count+n <= 0 {
				inv.Items.Delete(id)
				continue
			}

			inv.Items.Set(id, ucAddToStack(s, n, cmd.Payload.From))
		}

		e.Set(inv)

		changed[cmd.Payload.Who%ucPlayers] = true
		// This reader's copy. The pickup log system reads the same command later this tick.
		cmd.Payload.Items.Clear()
	}

	for who, ok := range changed {
		if !ok {
			continue
		}

		inv := ucFind(w, uint8(who)).Get[ucInventory]()
		w.Broadcast(ucInventoryChanged{Who: uint8(who), Items: inv.Items})
		// Edits after Broadcast, through Slices nested in the values, must not reach the event.
		for id, s := range inv.Items.All() {
			s.From.Set(0, "after broadcast")
			s.Count = -1
			inv.Items.Set(id, s)
		}

		inv.Items.Set(ucNoItem, ucStack{Count: 1})
	}
}

// ucAbilitySystem expires cooldowns by deleting while ranging, as with a Go map, then casts an
// ability when it is off cooldown and tells the caster every cooldown it now has.
type ucAbilitySystem struct{}

func (*ucAbilitySystem) Run(w *World) {
	tick := w.TickHeight()
	for e := range w.Contains[ucRow]().Iter() {
		cd := e.Get[ucCooldowns]()
		for ability, at := range cd.ReadyAt.All() {
			if at <= tick {
				cd.ReadyAt.Delete(ability)
			}
		}

		e.Set(cd)
	}

	for cmd := range w.Commands[ucCast]() {
		p := cmd.Payload
		e := ucFind(w, p.Who)

		cd, l := e.Get[ucCooldowns](), e.Get[ucLog]()
		if cd.ReadyAt.Has(p.Ability) {
			ucAppendLog(&l, tick, "blocked "+p.Ability)
		} else {
			cd.ReadyAt.Set(p.Ability, tick+uint64(p.Cooldown)+1)
			ucAppendLog(&l, tick, "cast "+p.Ability)
			w.SendTo(ucRecipient(p.Who), ucCastResult{Ability: p.Ability, ReadyAt: cd.ReadyAt})
		}

		e.Set(cd)
		e.Set(l)

		for k := range cd.ReadyAt.Keys() {
			cd.ReadyAt.Set(k, 0) // after SendTo and Set, so it must reach neither
		}
	}
}

// ucQuestSystem adds steps to a quest and, once it has ucQuestAt of them, removes the quest and
// emits ucQuestFinished.
type ucQuestSystem struct{}

func (*ucQuestSystem) Run(w *World) {
	for cmd := range w.Commands[ucProgress]() {
		p := cmd.Payload
		e := ucFind(w, p.Who)
		q := e.Get[ucQuests]()

		steps, _ := q.Steps.Get(p.Quest)
		for v := range p.Steps.Values() {
			steps.Append(v)
		}

		if steps.Len() < ucQuestAt {
			q.Steps.Set(p.Quest, steps)
			e.Set(q)

			continue
		}

		q.Steps.Delete(p.Quest)
		e.Set(q)

		done := ucQuestFinished{Who: p.Who % ucPlayers, Quest: p.Quest, Steps: steps}
		w.EmitSystemEvent(done)
		done.Steps.Set(0, 0xEE) // the emitter keeps editing its own value
	}
}

// ucRewardSystem grants an item for each finished quest, then edits the system event it read.
type ucRewardSystem struct{}

func (*ucRewardSystem) Run(w *World) {
	tick := w.TickHeight()
	for done := range w.SystemEvents[ucQuestFinished]() {
		e := ucFind(w, done.Who)
		inv, l := e.Get[ucInventory](), e.Get[ucLog]()
		s, _ := inv.Items.Get(ucReward)
		n := int32(done.Steps.Len())
		inv.Items.Set(ucReward, ucAddToStack(s, n, "quest "+done.Quest))
		ucAppendLog(&l, tick, "reward "+done.Quest)
		e.Set(inv)
		e.Set(l)
		// This reader's copy. The audit system reads the same event after this one.
		done.Steps.Set(0, 0xDD)
		done.Steps.Reset()
	}
}

// ucAuditSystem tells clients about every finished quest, with the steps that finished it.
type ucAuditSystem struct{}

func (*ucAuditSystem) Run(w *World) {
	for done := range w.SystemEvents[ucQuestFinished]() {
		w.Broadcast(ucQuestDone(done))
	}
}

// ucSkillSystem appends ranks to a skill in a two-level tree, keeping the last ucRankCap.
type ucSkillSystem struct{}

func (*ucSkillSystem) Run(w *World) {
	for cmd := range w.Commands[ucLearn]() {
		p := cmd.Payload
		e := ucFind(w, p.Who)
		sk := e.Get[ucSkills]()
		branch, _ := sk.Tree.Get(p.Branch)

		ranks, _ := branch.Get(p.Skill)
		for r := range p.Ranks.Values() {
			ranks.Append(r)
		}

		if n := ranks.Len(); n > ucRankCap {
			ranks.Delete(0, n-ucRankCap)
		}

		branch.Set(p.Skill, ranks)
		sk.Tree.Set(p.Branch, branch)
		e.Set(sk)
	}
}

// ucPartySystem disbands every party each fifth tick, then adds members in join order and records
// the roles each one takes.
type ucPartySystem struct{}

func (*ucPartySystem) Run(w *World) {
	if w.TickHeight()%5 == 0 {
		for e := range w.Contains[ucRow]().Iter() {
			p := e.Get[ucParty]()
			p.Members.Reset()
			e.Set(p)
		}
	}

	for cmd := range w.Commands[ucJoin]() {
		e := ucFind(w, cmd.Payload.Who)
		p := e.Get[ucParty]()

		i := p.Members.IndexFunc(func(m ucMember) bool { return m.Name == cmd.Payload.Member })
		if i < 0 {
			p.Members.Append(ucMember{Name: cmd.Payload.Member})
			i = p.Members.Len() - 1
		}

		m := p.Members.At(i)
		m.Roles.Append(cmd.Payload.Role)

		if n := m.Roles.Len(); n > ucRoleCap {
			m.Roles.Delete(0, n-ucRoleCap)
		}

		p.Members.Set(i, m)
		e.Set(p)
		// Edits after Set, including through the Slice nested in a member, must not reach the world.
		m.Roles.Set(0, "after set")
		p.Members.Append(ucMember{Name: "after set"})
	}
}

// ucPickupLogSystem is the second reader of ucPickup. The inventory system emptied its own copy of
// each command's Items, which must not change what this one reads.
type ucPickupLogSystem struct{}

func (*ucPickupLogSystem) Run(w *World) {
	tick := w.TickHeight()
	for cmd := range w.Commands[ucPickup]() {
		e := ucFind(w, cmd.Payload.Who)
		l := e.Get[ucLog]()
		ucAppendLog(&l, tick, fmt.Sprintf("picked %d from %s", cmd.Payload.Items.Len(), cmd.Payload.From))
		e.Set(l)
	}
}

type (
	ucPeekBefore struct{}
	ucPeekAfter  struct{}
)

func (*ucPeekBefore) Run(w *World) { ucPeek(w) }
func (*ucPeekAfter) Run(w *World)  { ucPeek(w) }

// ucPeek edits every collection it reads, at every level, and never calls Set.
func ucPeek(w *World) {
	for cmd := range w.Commands[ucPickup]() {
		for id := range cmd.Payload.Items.Keys() {
			cmd.Payload.Items.Set(id, -1000)
		}
	}

	for cmd := range w.Commands[ucProgress]() {
		if cmd.Payload.Steps.Len() > 0 {
			cmd.Payload.Steps.Set(0, 0xEE)
		}
	}

	for e := range w.Contains[ucRow]().Iter() {
		inv := e.Get[ucInventory]()
		for id, s := range inv.Items.All() {
			s.From.Set(0, "peek")
			s.Count = -7
			inv.Items.Set(id, s)
		}

		inv.Items.Set(ucNoItem, ucStack{Count: 1})

		l := e.Get[ucLog]()
		if l.Entries.Len() > 0 {
			l.Entries.Set(0, ucAction{What: "peek"})
		}

		l.Entries.Reverse()

		cd := e.Get[ucCooldowns]()
		for k := range cd.ReadyAt.Keys() {
			cd.ReadyAt.Set(k, 0)
		}

		cd.ReadyAt.Set("peek", 1)

		q := e.Get[ucQuests]()
		for name, steps := range q.Steps.All() {
			if steps.Len() > 0 {
				steps.Set(0, 0xEE)
			}

			steps.Append(0xEE)
			q.Steps.Set(name, steps)
		}

		sk := e.Get[ucSkills]()
		for b, branch := range sk.Tree.All() {
			for s, ranks := range branch.All() {
				if ranks.Len() > 0 {
					ranks.Set(0, -1)
				}

				branch.Set(s, ranks)
			}

			branch.Set("peek", immutable.SliceOf[int32](1))
			sk.Tree.Set(b, branch)
		}

		p := e.Get[ucParty]()
		for i, m := range p.Members.All() {
			m.Roles.Set(0, "peek")
			m.Name = "peek"
			p.Members.Set(i, m)
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Model: the same game on plain Go slices and maps
// -------------------------------------------------------------------------------------------------

type ucModelStack struct {
	Count int32
	From  []string
}

type ucModelPlayer struct {
	Items   map[ucItemID]ucModelStack
	Log     []ucAction
	ReadyAt map[string]uint64
	Quests  map[string][]uint8
	Tree    map[string]map[string][]int32
	Members []ucModelMember
}

type ucModelMember struct {
	Name  string
	Roles []string
}

func newUCModelPlayer() ucModelPlayer {
	return ucModelPlayer{
		Items:   map[ucItemID]ucModelStack{},
		ReadyAt: map[string]uint64{},
		Quests:  map[string][]uint8{},
		Tree:    map[string]map[string][]int32{},
	}
}

type ucModelCast struct {
	Recipient string
	Ability   string
	ReadyAt   map[string]uint64
}

type ucModelQuest struct {
	Who   uint8
	Quest string
	Steps []uint8
}

// ucModelEvents is what a tick sends, in the order the systems send it.
type ucModelEvents struct {
	Inventories map[uint8]map[ucItemID]ucModelStack
	Casts       []ucModelCast
	Quests      []ucModelQuest
}

type ucModel struct{ players [ucPlayers]ucModelPlayer }

func newUCModel() *ucModel {
	m := &ucModel{}
	for i := range m.players {
		m.players[i] = newUCModelPlayer()
	}

	return m
}

func lastN[T any](s []T, n int) []T { return s[max(0, len(s)-n):] }

func ucModelLog(p *ucModelPlayer, tick uint64, what string) {
	p.Log = lastN(append(slices.Clone(p.Log), ucAction{Tick: tick, What: what}), ucLogCap)
}

func ucModelAdd(s ucModelStack, n int32, from string) ucModelStack {
	return ucModelStack{Count: s.Count + n, From: lastN(append(slices.Clone(s.From), from), ucFromCap)}
}

// step runs one tick of the game in the order ucRegister registers its systems.
func (m *ucModel) step(tick uint64, in ucInputs) ucModelEvents {
	ev := ucModelEvents{Inventories: map[uint8]map[ucItemID]ucModelStack{}}

	for _, c := range in.pickups {
		p := &m.players[c.Who%ucPlayers]
		for id, n := range c.Items.All() {
			if p.Items[id].Count+n <= 0 {
				delete(p.Items, id)
				continue
			}

			p.Items[id] = ucModelAdd(p.Items[id], n, c.From)
		}

		ev.Inventories[c.Who%ucPlayers] = maps.Clone(p.Items)
	}

	for i := range m.players {
		maps.DeleteFunc(m.players[i].ReadyAt, func(_ string, at uint64) bool { return at <= tick })
	}

	for _, c := range in.casts {
		p := &m.players[c.Who%ucPlayers]
		if _, ok := p.ReadyAt[c.Ability]; ok {
			ucModelLog(p, tick, "blocked "+c.Ability)
			continue
		}

		p.ReadyAt[c.Ability] = tick + uint64(c.Cooldown) + 1
		ucModelLog(p, tick, "cast "+c.Ability)
		ev.Casts = append(ev.Casts, ucModelCast{ucRecipient(c.Who), c.Ability, maps.Clone(p.ReadyAt)})
	}

	for _, c := range in.progress {
		p := &m.players[c.Who%ucPlayers]

		steps := append(slices.Clone(p.Quests[c.Quest]), slices.Collect(c.Steps.Values())...)
		if len(steps) < ucQuestAt {
			p.Quests[c.Quest] = steps
			continue
		}

		delete(p.Quests, c.Quest)
		ev.Quests = append(ev.Quests, ucModelQuest{c.Who % ucPlayers, c.Quest, steps})
	}

	for _, q := range ev.Quests {
		p := &m.players[q.Who]
		p.Items[ucReward] = ucModelAdd(
			p.Items[ucReward],
			int32(len(q.Steps)),
			"quest "+q.Quest,
		)
		ucModelLog(p, tick, "reward "+q.Quest)
	}

	for _, c := range in.learn {
		p := &m.players[c.Who%ucPlayers]
		if p.Tree[c.Branch] == nil {
			p.Tree[c.Branch] = map[string][]int32{}
		}

		ranks := append(slices.Clone(p.Tree[c.Branch][c.Skill]), slices.Collect(c.Ranks.Values())...)
		p.Tree[c.Branch][c.Skill] = lastN(ranks, ucRankCap)
	}

	if tick%5 == 0 {
		for i := range m.players {
			m.players[i].Members = nil
		}
	}

	for _, c := range in.joins {
		p := &m.players[c.Who%ucPlayers]
		p.Members = slices.Clone(p.Members)

		i := slices.IndexFunc(p.Members, func(m ucModelMember) bool { return m.Name == c.Member })
		if i < 0 {
			p.Members = append(p.Members, ucModelMember{Name: c.Member})
			i = len(p.Members) - 1
		}

		p.Members[i].Roles = lastN(append(slices.Clone(p.Members[i].Roles), c.Role), ucRoleCap)
	}

	for _, c := range in.pickups {
		ucModelLog(&m.players[c.Who%ucPlayers], tick, fmt.Sprintf("picked %d from %s", c.Items.Len(), c.From))
	}

	return ev
}

// -------------------------------------------------------------------------------------------------
// Reading the world back as model values
// -------------------------------------------------------------------------------------------------

func ucGoMap[K immutable.Key, V, W any](m immutable.Map[K, V], conv func(V) W) map[K]W {
	out := make(map[K]W, m.Len())
	for k, v := range m.All() {
		out[k] = conv(v)
	}

	return out
}

// ucGoSlice returns nil for an empty Slice, as the model holds an emptied slice.
func ucGoSlice[T any](s immutable.Slice[T]) []T { return slices.Collect(s.Values()) }

func ucGoItems(items immutable.Map[ucItemID, ucStack]) map[ucItemID]ucModelStack {
	return ucGoMap(items, func(s ucStack) ucModelStack { return ucModelStack{s.Count, ucGoSlice(s.From)} })
}

func ucGoMembers(members immutable.Slice[ucMember]) []ucModelMember {
	var out []ucModelMember
	for m := range members.Values() {
		out = append(out, ucModelMember{m.Name, ucGoSlice(m.Roles)})
	}

	return out
}

func ucSame[T any](v T) T { return v }

func ucReadPlayers(t *testing.T, w *World) [ucPlayers]ucModelPlayer {
	t.Helper()

	var got [ucPlayers]ucModelPlayer

	n := 0
	for e := range w.Contains[ucRow]().Iter() {
		n++
		got[e.Get[ucPlayer]().Who] = ucModelPlayer{
			Items:   ucGoItems(e.Get[ucInventory]().Items),
			Log:     ucGoSlice(e.Get[ucLog]().Entries),
			ReadyAt: ucGoMap(e.Get[ucCooldowns]().ReadyAt, ucSame[uint64]),
			Quests:  ucGoMap(e.Get[ucQuests]().Steps, ucGoSlice[uint8]),
			Tree: ucGoMap(
				e.Get[ucSkills]().Tree,
				func(b immutable.Map[string, immutable.Slice[int32]]) map[string][]int32 {
					return ucGoMap(b, ucGoSlice[int32])
				},
			),
			Members: ucGoMembers(e.Get[ucParty]().Members),
		}
	}

	require.Equal(t, ucPlayers, n, "players")

	return got
}

func ucReadEvents(w *TestWorld) ucModelEvents {
	got := ucModelEvents{Inventories: map[uint8]map[ucItemID]ucModelStack{}}
	for _, sent := range w.Events[ucInventoryChanged]() {
		got.Inventories[sent.Payload.Who] = ucGoItems(sent.Payload.Items)
	}

	for _, sent := range w.Events[ucCastResult]() {
		got.Casts = append(got.Casts, ucModelCast{
			sent.Recipient, sent.Payload.Ability, ucGoMap(sent.Payload.ReadyAt, ucSame[uint64]),
		})
	}

	for _, sent := range w.Events[ucQuestDone]() {
		got.Quests = append(
			got.Quests,
			ucModelQuest{sent.Payload.Who, sent.Payload.Quest, ucGoSlice(sent.Payload.Steps)},
		)
	}

	return got
}

// -------------------------------------------------------------------------------------------------
// Inputs
// -------------------------------------------------------------------------------------------------

type ucInputs struct {
	pickups  []ucPickup
	casts    []ucCast
	progress []ucProgress
	learn    []ucLearn
	joins    []ucJoin
}

func ucPick(rng *rand.Rand, options ...string) string { return options[rng.IntN(len(options))] }

// ucRandomInputs builds a tick's commands. Who ranges past the last player to cover the wrap, and
// every Map is built from a Go map, whose order differs between runs.
func ucRandomInputs(rng *rand.Rand) ucInputs {
	var in ucInputs

	who := func() uint8 { return uint8(rng.IntN(ucPlayers + 1)) }

	for range rng.IntN(3) {
		items := map[ucItemID]int32{}
		for range rng.IntN(4) {
			items[ucItemID(rng.IntN(6))] = int32(rng.IntN(9) - 3)
		}

		in.pickups = append(in.pickups, ucPickup{
			Who: who(), From: ucPick(rng, "chest", "boss", "shop"), Items: immutable.MapFrom(maps.All(items)),
		})
	}

	for range rng.IntN(3) {
		in.casts = append(in.casts, ucCast{
			Who:      who(),
			Ability:  ucPick(rng, "fire", "ice", "heal"),
			Cooldown: uint8(rng.IntN(4)),
		})
	}

	for range rng.IntN(3) {
		var steps immutable.Slice[uint8]
		for range rng.IntN(4) {
			steps.Append(uint8(rng.IntN(200)))
		}

		in.progress = append(in.progress, ucProgress{Who: who(), Quest: ucPick(rng, "q1", "q2", "q3"), Steps: steps})
	}

	for range rng.IntN(2) {
		var ranks immutable.Slice[int32]
		for range rng.IntN(4) {
			ranks.Append(int32(rng.IntN(10)))
		}

		in.learn = append(in.learn, ucLearn{
			Who: who(), Branch: ucPick(rng, "war", "magic"), Skill: ucPick(rng, "a", "b", "c"), Ranks: ranks,
		})
	}

	for range rng.IntN(2) {
		in.joins = append(in.joins, ucJoin{
			Who: who(), Member: ucPick(rng, "ann", "ben", "cid", "dee"), Role: ucPick(rng, "tank", "heal", "dps"),
		})
	}

	return in
}

func (in ucInputs) send(w *TestWorld) {
	for _, c := range in.pickups {
		w.Command("client", c)
	}

	for _, c := range in.casts {
		w.Command("client", c)
	}

	for _, c := range in.progress {
		w.Command("client", c)
	}

	for _, c := range in.learn {
		w.Command("client", c)
	}

	for _, c := range in.joins {
		w.Command("client", c)
	}
}

// -------------------------------------------------------------------------------------------------
// Tests
// -------------------------------------------------------------------------------------------------

func ucNewWorld(t *testing.T) *TestWorld {
	t.Helper()
	w := NewTestWorld(t, ucRegister)
	w.useSyncSnapshotStorage(&memSnapshotStorage{t: t})

	return w
}

// TestImmutableUseCases_MatchModelAcrossRestores checks the world against the model after every tick.
// Every tenth tick it also restores a new world from the live world's latest snapshot; each restored
// world then gets the same commands, and must match the model and encode to the live world's bytes on
// every tick after.
func TestImmutableUseCases_MatchModelAcrossRestores(t *testing.T) {
	t.Parallel()

	const ticks, restoreEvery = 80, 10

	rng := rand.New(rand.NewPCG(1, 2))
	model := newUCModel()
	live := ucNewWorld(t)
	worlds := []*TestWorld{live}

	var sawQuest, sawRemoval, sawBlocked bool

	for step := range ticks {
		if step > 0 && step%restoreEvery == 0 {
			restored := NewTestWorld(t, ucRegister)
			restored.useSyncSnapshotStorage(live.snapshotStorage)
			require.NoError(t, restored.restore(context.Background()))
			restored.useSyncSnapshotStorage(&memSnapshotStorage{t: t})
			require.Equal(t, live.TickHeight(), restored.TickHeight())
			require.Equal(t, model.players, ucReadPlayers(t, restored.World), "restored at step %d", step)
			worlds = append(worlds, restored)
		}

		in := ucRandomInputs(rng)

		var stacks int
		for _, p := range model.players {
			stacks += len(p.Items)
		}

		want := model.step(live.TickHeight(), in)
		sawQuest = sawQuest || len(want.Quests) > 0

		sawBlocked = sawBlocked || len(in.casts) > len(want.Casts)
		for _, p := range model.players {
			stacks -= len(p.Items)
		}

		sawRemoval = sawRemoval || stacks > 0

		for i, w := range worlds {
			in.send(w)
			w.Tick()
			require.Equal(t, model.players, ucReadPlayers(t, w.World), "world %d, step %d", i, step)
			require.Equal(t, want, ucReadEvents(w), "events of world %d, step %d", i, step)
			require.Equal(t, live.world.EncodeState(nil), w.world.EncodeState(nil), "world %d, step %d", i, step)
		}
	}

	// The script must actually reach the paths the systems branch on.
	require.True(t, sawQuest, "a quest finished")
	require.True(t, sawRemoval, "a stack was removed")
	require.True(t, sawBlocked, "a cast was blocked by its cooldown")
}

// TestImmutableUseCases_Deterministic runs the game twice. Commands carry Maps built from Go maps,
// whose order differs between the runs, and the state and events must still match after every step.
func TestImmutableUseCases_Deterministic(t *testing.T) {
	t.Parallel()
	RequireDeterministic(t, ucRegister, func(w *TestWorld) {
		rng := rand.New(rand.NewPCG(3, 4))
		for range 80 {
			ucRandomInputs(rng).send(w)
			w.Tick()
		}
	})
}

// TestImmutableUseCases_DST runs the game under DST, which fills every command's collections with
// random values and snapshots, restores and restarts the world at random.
func TestImmutableUseCases_DST(t *testing.T) { //nolint:paralleltest // RunDST sets environment variables.
	RunDST(t, ucRegister, nil)
}
