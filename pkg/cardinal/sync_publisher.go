package cardinal

import (
	"bytes"
	"maps"
	"math/bits"
	"slices"
	"strconv"
	"strings"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/kelindar/bitmap"
	"github.com/vmihailenco/msgpack/v5"
)

// The publisher keeps every reader's replica equal to its slice of the world. Three nouns:
//
//   - An entry is what is true about one entity lifetime: its record, encoded once, and the
//     policy that says who may view it and who has authority. Entries are immutable.
//   - The log is the tick's ordered list of transitions (an entity's old entry and new entry),
//     interleaved with its events, then the readers that open or close.
//   - What a reader holds is not stored. An open reader held an entity exactly when the entity's
//     previous entry gave it access, so a transition carries everything the fold needs.
//
// Each reader that has a stream holds a slot, so an audience is a bitset over slots. The tick
// goroutine owns all of it; see reconcileSync for the one place that reads the service.

// bitset is a set of small integers. Missing words read as zero.
type bitset []uint64

func (b *bitset) word(w int) uint64 {
	if w < len(*b) {
		return (*b)[w]
	}
	return 0
}

func (b *bitset) has(i int) bool { return b.word(i>>6)&(1<<(i&63)) != 0 }

func (b *bitset) put(i int, on bool) {
	w := i >> 6
	if !on {
		if w < len(*b) {
			(*b)[w] &^= 1 << (i & 63)
		}
		return
	}
	for len(*b) <= w {
		*b = append(*b, 0)
	}
	(*b)[w] |= 1 << (i & 63)
}

func (b *bitset) or(o bitset) {
	for len(*b) < len(o) {
		*b = append(*b, 0)
	}
	for w, word := range o {
		(*b)[w] |= word
	}
}

// entitySet is a set of entity IDs whose iteration costs its size, not its capacity.
type entitySet struct {
	in   bitset
	list []EntityID
}

func (s *entitySet) add(n EntityID) {
	if !s.in.has(int(n)) {
		s.in.put(int(n), true)
		s.list = append(s.list, n)
	}
}

func (s *entitySet) take() []EntityID {
	out := s.list
	for _, n := range out {
		s.in.put(int(n), false)
	}
	s.list = nil
	return out
}

// -------------------------------------------------------------------------------------------------
// Roster: each reader with a sync stream has a dense slot.
// -------------------------------------------------------------------------------------------------

type syncRoster struct {
	readers []syncReader // by slot
	slotOf  map[string]int
	live    bitset // slots whose stream is open at the current position in the log
	free    []int
	cooling []int // left this tick; reusable once the tick's log has been dispatched
}

// syncReader is one slot. Only the log's open and close items change its stream fields, so a
// transition already queued reaches the stream that was open when it was queued.
type syncReader struct {
	player     string // "" while the slot is free
	subscriber *streamSubscriber
	generation uint64 // the world_sync subscription the stream was opened for
	stream     string
	sequence   uint64
}

func (r *syncRoster) leave(player string) {
	if s, ok := r.slotOf[player]; ok {
		r.cooling = append(r.cooling, s)
	}
}

func (r *syncRoster) release() {
	for _, s := range r.cooling {
		delete(r.slotOf, r.readers[s].player)
		r.readers[s] = syncReader{}
		r.free = append(r.free, s)
	}
	r.cooling = r.cooling[:0]
}

// -------------------------------------------------------------------------------------------------
// Groups and policies
// -------------------------------------------------------------------------------------------------

// syncGroup is an immutable snapshot of one sync.Group. A membership change makes a new one.
type syncGroup struct {
	serial  uint64 // identity, for the policy key
	members map[string]struct{}
	mask    bitset // members who hold a slot
}

// syncList is a sync.Viewers or sync.Authorities as written.
type syncList struct{ players, groups []string }

// resolvedList is a syncList with its players sorted and its group names resolved.
type resolvedList struct {
	players []string
	groups  []*syncGroup // nil: no group, or more than one, has that name
}

func (l resolvedList) has(player string) bool {
	if _, ok := slices.BinarySearch(l.players, player); ok {
		return true
	}
	for _, g := range l.groups {
		if g == nil {
			continue
		}
		if _, ok := g.members[player]; ok {
			return true
		}
	}
	return false
}

// syncPolicy is one interned pair of access lists. Entities with equal lists share one, so
// "did access change?" is a pointer comparison.
type syncPolicy struct {
	key      string
	src      [2]*syncList    // viewers, authorities as written; nil: the entity has no such component
	lists    [2]resolvedList // the same, resolved
	stale    bool            // a group it names was replaced: its entities resolve their lists again
	view     bitset          // slots named by viewers
	auth     bitset          // slots named by authorities
	entities []EntityID      // entities whose current entry uses this policy
}

// words returns one word of "may view" and "has authority", indexed by slot.
func (p *syncPolicy) words(w int) (uint64, uint64) {
	auth := p.auth.word(w)
	if p.src[0] == nil {
		return ^uint64(0), auth // no sync.Viewers: every reader may view
	}
	return p.view.word(w) | auth, auth
}

// access is the definition, and the path for a player without a slot.
func (p *syncPolicy) access(player string) syncstate.Access {
	switch {
	case p.lists[1].has(player):
		return syncstate.Authority
	case p.src[0] == nil || p.lists[0].has(player):
		return syncstate.View
	}
	return syncstate.None
}

func (p *syncPolicy) bind(s int, player string) {
	p.view.put(s, p.lists[0].has(player))
	p.auth.put(s, p.lists[1].has(player))
}

func (l *syncList) groupNames() []string {
	if l == nil {
		return nil
	}
	return l.groups
}

// written reports whether a component as it is now equals what the policy was built from.
func (l *syncList) written(present bool, players, groups immutable.Slice[string]) bool {
	if l == nil || !present {
		return l == nil && !present
	}
	return sameStrings(l.players, players) && sameStrings(l.groups, groups)
}

func sameStrings(a []string, b immutable.Slice[string]) bool {
	if len(a) != b.Len() {
		return false
	}
	for i, v := range a {
		if b.At(i) != v {
			return false
		}
	}
	return true
}

// -------------------------------------------------------------------------------------------------
// Entries and transitions
// -------------------------------------------------------------------------------------------------

type syncEntry struct {
	id       syncstate.ID
	revision uint64
	presence uint64
	values   []syncstate.Value  // the state, compared by value
	record   msgpack.RawMessage // the same state as a sync.Record, encoded once for every reader
	policy   *syncPolicy
}

// syncTransition is one log item. It carries the entry it replaces, so what each reader held
// before is derived from it and no per-reader rows are stored.
type syncTransition struct {
	entity   EntityID
	old, now *syncEntry // nil: no entry
}

// accessLists are an entity's access components as read from the world.
type accessLists struct {
	hasViewers, hasAuthorities bool
	viewers                    syncstate.Viewers
	authorities                syncstate.Authorities
}

// -------------------------------------------------------------------------------------------------
// Publisher
// -------------------------------------------------------------------------------------------------

type syncPublisher struct {
	admit       func(string) bool // nil until EnableSync: nothing is tracked or sent
	components  map[ecs.ComponentID]syncstate.Component
	names       []string // sorted names of components; nil until first needed
	marker      ecs.ComponentID
	group       ecs.ComponentID
	viewers     ecs.ComponentID
	authorities ecs.ComponentID
	fingerprint string

	presence     map[EntityID]map[ecs.ComponentID]uint64
	dirty        entitySet             // the one hint: entities to look at again
	deferred     []EntityID            // new lifetimes waiting for the end of the tick
	groupsDirty  bool                  // a sync.Group was written, added, removed or destroyed
	groupHolders map[EntityID]struct{} // entities that held a sync.Group at the last refresh

	entries  []*syncEntry // by entity ID
	pos      []int        // by entity ID: its index in its policy's entities
	groups   map[string]*syncGroup
	policies map[string]*syncPolicy
	serial   uint64
	roster   syncRoster
	retired  map[syncstate.ID]*syncEntry // entries removed this tick, for events about their lifetimes
}

func (p *syncPublisher) enabled() bool { return p != nil && p.admit != nil }

func (p *syncPublisher) entry(n EntityID) *syncEntry {
	if int(n) < len(p.entries) {
		return p.entries[n]
	}
	return nil
}

// put is the only writer of entries, so it is the only writer of policy.entities.
func (p *syncPublisher) put(n EntityID, now *syncEntry) {
	for len(p.entries) <= int(n) {
		p.entries = append(p.entries, nil)
		p.pos = append(p.pos, 0)
	}
	old := p.entries[n]
	if old != nil && (now == nil || now.policy != old.policy) {
		list, i := old.policy.entities, p.pos[n]
		last := list[len(list)-1]
		list[i], p.pos[last] = last, i
		old.policy.entities = list[:len(list)-1]
	}
	if now != nil && (old == nil || now.policy != old.policy) {
		p.pos[n] = len(now.policy.entities)
		now.policy.entities = append(now.policy.entities, n)
	}
	p.entries[n] = now
}

// observe records which entities to look at again. It runs inside ECS writes, so it reads
// nothing from the world.
func (p *syncPublisher) observe(m ecs.Mutation) {
	if p.admit == nil {
		return
	}
	switch m.Kind {
	case ecs.Reset:
		for n, e := range p.entries {
			if e != nil {
				p.dirty.add(EntityID(n))
			}
		}
		clear(p.presence)
		p.groupsDirty = true
	case ecs.Created, ecs.Destroyed:
		p.dirty.add(m.Entity)
		delete(p.presence, m.Entity)
		if _, held := p.groupHolders[m.Entity]; held {
			p.groupsDirty = true
		}
	case ecs.ComponentSet, ecs.ComponentAdded, ecs.ComponentRemoved:
		_, tracked := p.components[m.Component]
		switch {
		case m.Component == p.group:
			p.groupsDirty = true
			return
		case tracked || m.Component == p.marker:
			if m.Kind != ecs.ComponentSet {
				p.bumpPresence(m.Entity, m.Component)
			}
		case m.Component != p.viewers && m.Component != p.authorities:
			return
		}
		p.dirty.add(m.Entity)
	}
}

func (p *syncPublisher) bumpPresence(n EntityID, c ecs.ComponentID) {
	if p.presence[n] == nil {
		p.presence[n] = map[ecs.ComponentID]uint64{}
	}
	p.presence[n][c]++
}

// intern returns the policy for a pair of lists as written, resolved against today's groups.
// Its key is both lists as written plus the identity of each group they resolve to.
func (p *syncPublisher) intern(src [2]*syncList) *syncPolicy {
	var key strings.Builder
	var lists [2]resolvedList
	for i, l := range src {
		lists[i] = l.resolve(p.groups)
		writeKey(&key, l, lists[i])
	}
	if pol := p.policies[key.String()]; pol != nil {
		pol.stale = false // the key says its groups are today's
		return pol
	}
	pol := &syncPolicy{key: key.String(), src: src, lists: lists}
	p.mask(&pol.view, src[0], lists[0])
	p.mask(&pol.auth, src[1], lists[1])
	p.policies[pol.key] = pol
	return pol
}

// resolve sorts a list's players and resolves its group names against groups, where a name
// that is missing has no members.
func (l *syncList) resolve(groups map[string]*syncGroup) resolvedList {
	var out resolvedList
	if l == nil {
		return out
	}
	out.players = slices.Sorted(slices.Values(l.players))
	for _, name := range l.groups {
		out.groups = append(out.groups, groups[name])
	}
	return out
}

// writeKey writes a list's part of a policy key: the list as written, and the identity of each
// group it resolved to.
func writeKey(key *strings.Builder, l *syncList, resolved resolvedList) {
	if l == nil {
		key.WriteString("-\x03")
		return
	}
	for _, player := range l.players {
		key.WriteString(player)
		key.WriteByte('\x00')
	}
	key.WriteByte('\x01')
	for j, name := range l.groups {
		key.WriteString(name)
		key.WriteByte('\x02')
		if g := resolved.groups[j]; g != nil {
			key.WriteString(strconv.FormatUint(g.serial, 36))
		}
		key.WriteByte('\x00')
	}
	key.WriteByte('\x03')
}

// mask sets the slot bit of every seated reader a list names, directly or through a group.
func (p *syncPublisher) mask(bits *bitset, l *syncList, resolved resolvedList) {
	if l == nil {
		return
	}
	for _, player := range l.players {
		if s, ok := p.roster.slotOf[player]; ok {
			bits.put(s, true)
		}
	}
	for _, g := range resolved.groups {
		if g != nil {
			bits.or(g.mask)
		}
	}
}

// policyOf returns the policy for an entity's lists. They are resolved again only when they
// differ from the old policy's as written, or a group that policy names was replaced.
func (p *syncPublisher) policyOf(lists accessLists, old *syncEntry) *syncPolicy {
	if old != nil && !old.policy.stale &&
		old.policy.src[0].written(lists.hasViewers, lists.viewers.Players, lists.viewers.Groups) &&
		old.policy.src[1].written(lists.hasAuthorities, lists.authorities.Players, lists.authorities.Groups) {
		return old.policy
	}
	return p.intern(lists.sources())
}

// sources returns the lists as written: viewers, then authorities. nil: no such component.
func (l *accessLists) sources() [2]*syncList {
	var src [2]*syncList
	if l.hasViewers {
		src[0] = &syncList{stringsOf(l.viewers.Players), stringsOf(l.viewers.Groups)}
	}
	if l.hasAuthorities {
		src[1] = &syncList{stringsOf(l.authorities.Players), stringsOf(l.authorities.Groups)}
	}
	return src
}

func stringsOf(s immutable.Slice[string]) []string { return slices.Collect(s.Values()) }

// readLists reads an entity's access components and nothing else.
func readLists(w *World, n EntityID) accessLists {
	e := w.Entity(n)
	var lists accessLists
	if lists.hasViewers = e.Has[syncstate.Viewers](); lists.hasViewers {
		lists.viewers = e.Get[syncstate.Viewers]()
	}
	if lists.hasAuthorities = e.Has[syncstate.Authorities](); lists.hasAuthorities {
		lists.authorities = e.Get[syncstate.Authorities]()
	}
	return lists
}

// refreshGroups rebuilds the group snapshots. An unchanged group keeps its pointer, so policies
// that name it stay current. Only the entities of a policy that names a replaced group are
// looked at again.
func (p *syncPublisher) refreshGroups(w *World) {
	if !p.groupsDirty {
		return
	}
	p.groupsDirty = false
	live := readGroups(w)
	p.groupHolders = live.holders
	for _, name := range slices.Sorted(maps.Keys(live.duplicates)) {
		w.tel.Logger.Error().Str("group", name).Int("count", live.duplicates[name]).
			Msg("sync group name is not unique; the name has no members until one remains")
	}
	next := map[string]*syncGroup{}
	for _, name := range slices.Sorted(maps.Keys(live.members)) {
		next[name] = p.snapshot(p.groups[name], live.members[name])
	}
	p.groups = next
	for _, pol := range p.policies {
		for i, l := range pol.src {
			for j, name := range l.groupNames() {
				pol.stale = pol.stale || next[name] != pol.lists[i].groups[j]
			}
		}
		if pol.stale {
			for _, n := range pol.entities {
				p.dirty.add(n)
			}
		}
	}
}

// groupReading is every sync.Group in the world as it is at one moment.
type groupReading struct {
	members    map[string]map[string]struct{} // by name, for names exactly one group holds
	duplicates map[string]int                 // names more than one group holds: no members
	holders    map[EntityID]struct{}          // entities that hold a sync.Group
}

// readGroups reads every sync.Group in the world. It writes nothing, so the live authority
// lookup and the group refresh share one definition of a group's members.
func readGroups(w *World) groupReading {
	r := groupReading{members: map[string]map[string]struct{}{}, holders: map[EntityID]struct{}{}}
	count := map[string]int{}
	var mask bitmap.Bitmap
	mask.Set(w.sync.group)
	err := w.world.IterEntities(mask, ecs.MatchContains, func(n EntityID) bool {
		g := w.Entity(n).Get[syncstate.Group]()
		r.holders[n] = struct{}{}
		count[g.GroupName]++
		set := map[string]struct{}{}
		for player := range g.Players.Values() {
			set[player] = struct{}{}
		}
		r.members[g.GroupName] = set
		return true
	})
	if err != nil {
		panic(err)
	}
	for name, c := range count {
		if c > 1 {
			if r.duplicates == nil {
				r.duplicates = map[string]int{}
			}
			r.duplicates[name] = c
			delete(r.members, name)
		}
	}
	return r
}

// snapshot returns old if its members are unchanged, else a new snapshot with its own identity.
func (p *syncPublisher) snapshot(old *syncGroup, members map[string]struct{}) *syncGroup {
	if old != nil && maps.Equal(old.members, members) {
		return old
	}
	p.serial++
	g := &syncGroup{serial: p.serial, members: members}
	for player := range members {
		if s, ok := p.roster.slotOf[player]; ok {
			g.mask.put(s, true)
		}
	}
	return g
}

// lifetime returns the lifetime of a synced entity: alive, with the sync.Sync marker.
func (p *syncPublisher) lifetime(w *World, n EntityID) (syncstate.ID, bool) {
	e := w.Entity(n)
	if !e.Alive() || !e.Has[syncstate.Sync]() {
		return syncstate.ID{}, false
	}
	return e.SyncID(), true
}

// capture appends a transition for every dirty entity whose entry differs from its last one.
// Marking too much dirty is always safe: an equal entry publishes nothing.
func (p *syncPublisher) capture(w *World, final bool) []syncTransition {
	p.refreshGroups(w)
	ids := p.dirty.take()
	if final {
		ids = append(ids, p.deferred...)
		p.deferred = p.deferred[:0]
	}
	if len(ids) == 0 {
		return nil
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var out []syncTransition
	for _, n := range ids {
		old := p.entry(n)
		id, synced := p.lifetime(w, n)
		if old != nil && (!synced || id != old.id) { // gone or replaced
			out = append(out, syncTransition{entity: n, old: old})
			p.retired[old.id] = old
			p.put(n, nil)
			old = nil
		}
		if !synced {
			continue
		}
		if old == nil && !final {
			p.deferred = append(p.deferred, n) // a new lifetime waits for the end of the tick
			continue
		}
		now := p.read(w, n, id, old)
		switch {
		case old == nil:
			now.revision = 1
		case !sameState(old, now):
			now.revision = old.revision + 1
		case old.policy == now.policy:
			continue // nothing changed: one pointer comparison
		default:
			now.revision, now.record = old.revision, old.record // only access changed
		}
		if now.record == nil {
			now.record = encodeRecord(now)
		}
		p.put(n, now)
		out = append(out, syncTransition{entity: n, old: old, now: now})
	}
	return out
}

// read builds the entry of one synced entity: its synced components and its policy.
func (p *syncPublisher) read(w *World, n EntityID, id syncstate.ID, old *syncEntry) *syncEntry {
	if p.names == nil {
		p.names = []string{}
		for _, c := range p.components {
			p.names = append(p.names, c.Name)
		}
		slices.Sort(p.names)
	}
	data, err := w.world.ReadObserved(n, p.names)
	if err != nil {
		panic(err)
	}
	now := &syncEntry{
		id:       id,
		presence: max(1, p.presence[n][p.marker]),
		values:   []syncstate.Value{},
		policy:   p.policyOf(readLists(w, n), old),
	}
	for cid, c := range p.components {
		value, present := data[c.Name]
		if !present {
			continue
		}
		now.values = append(
			now.values,
			syncstate.Value{Name: c.Name, Presence: max(1, p.presence[n][cid]), Data: value},
		)
	}
	slices.SortFunc(now.values, func(a, b syncstate.Value) int { return strings.Compare(a.Name, b.Name) })
	return now
}

func sameState(a, b *syncEntry) bool {
	return a.presence == b.presence && slices.EqualFunc(a.values, b.values, func(x, y syncstate.Value) bool {
		return x.Name == y.Name && x.Presence == y.Presence && bytes.Equal(x.Data, y.Data)
	})
}

// encodeRecord encodes an entry's record once. Every reader's frame copies these bytes.
func encodeRecord(e *syncEntry) msgpack.RawMessage {
	data, err := msgpack.Marshal(syncstate.Record{
		ID: e.id, Revision: e.revision, Presence: e.presence, Components: e.values,
	})
	if err != nil {
		panic(err)
	}
	return data
}

// sweep drops policies no entry uses. It runs after dispatch, when no queued item or retired
// entry holds one.
func (p *syncPublisher) sweep() {
	for key, pol := range p.policies {
		if len(pol.entities) == 0 {
			delete(p.policies, key)
		}
	}
}

// seat gives a player a slot and computes that one bit in every group and policy.
func (p *syncPublisher) seat(player string) int {
	r := &p.roster
	if s, ok := r.slotOf[player]; ok {
		r.cooling = slices.DeleteFunc(r.cooling, func(c int) bool { return c == s })
		return s
	}
	s := len(r.readers)
	if len(r.free) > 0 {
		s, r.free = r.free[len(r.free)-1], r.free[:len(r.free)-1]
	} else {
		r.readers = append(r.readers, syncReader{})
	}
	r.readers[s].player, r.slotOf[player] = player, s
	for _, g := range p.groups {
		_, member := g.members[player]
		g.mask.put(s, member)
	}
	for _, pol := range p.policies {
		pol.bind(s, player)
	}
	return s
}

// access answers for any player: two bit tests with a slot, the lists without one.
func (p *syncPublisher) access(pol *syncPolicy, player string) syncstate.Access {
	if pol == nil {
		return syncstate.None
	}
	s, ok := p.roster.slotOf[player]
	if !ok {
		return pol.access(player)
	}
	hold, auth := pol.words(s >> 6)
	switch bit := uint64(1) << (s & 63); {
	case auth&bit != 0:
		return syncstate.Authority
	case hold&bit != 0:
		return syncstate.View
	}
	return syncstate.None
}

// judge decides a command at target, by what the server last told its clients.
func (p *syncPublisher) judge(sender string, admitted bool, target syncstate.ID) syncstate.Reason {
	e := p.entry(EntityID(target.Entity))
	if e == nil || e.id != target || !admitted {
		return syncstate.StaleTarget
	}
	switch p.access(e.policy, sender) {
	case syncstate.None:
		return syncstate.StaleTarget // never reveal an entity the sender cannot see
	case syncstate.View:
		return syncstate.NoAuthority
	case syncstate.Authority:
	}
	return ""
}

// liveAuthority lists, in ascending order, the lifetimes of the synced entities that player has
// authority over in the world as it is now. It resolves each sync.Authorities as publication
// does, against the groups as they are now, so it does not wait for a capture.
func liveAuthority(w *World, player string) []syncstate.ID {
	groups := map[string]*syncGroup{}
	for name, members := range readGroups(w).members {
		groups[name] = &syncGroup{members: members}
	}
	var held []EntityID
	var mask bitmap.Bitmap
	mask.Set(w.sync.marker)
	mask.Set(w.sync.authorities)
	err := w.world.IterEntities(mask, ecs.MatchContains, func(n EntityID) bool {
		held = append(held, n)
		return true
	})
	if err != nil {
		panic(err)
	}
	slices.Sort(held)
	var out []syncstate.ID
	for _, n := range held {
		lists := readLists(w, n)
		if lists.sources()[1].resolve(groups).has(player) {
			out = append(out, w.Entity(n).SyncID())
		}
	}
	return out
}

// -------------------------------------------------------------------------------------------------
// The fold
// -------------------------------------------------------------------------------------------------

// syncDelta is what one reader is told about one commit.
type syncDelta struct {
	records []msgpack.RawMessage
	removed []syncstate.ID
	access  []syncstate.Grant
}

func (d *syncDelta) empty() bool { return len(d.records)+len(d.removed)+len(d.access) == 0 }

// fold works out who is told what for one commit; deltas is indexed by slot. A transition costs
// one pass over the words of open slots, plus its deliveries.
func (p *syncPublisher) fold(commit []syncTransition, deltas []syncDelta) {
	for _, t := range commit {
		for w, open := range p.roster.live {
			if open != 0 {
				foldWord(t, w, open, deltas)
			}
		}
	}
}

func foldWord(t syncTransition, w int, open uint64, deltas []syncDelta) {
	var h0, a0, h1, a1 uint64 // held before, authority before, holds now, authority now
	if t.old != nil {
		h0, a0 = t.old.policy.words(w)
	}
	if t.now != nil {
		h1, a1 = t.now.policy.words(w)
	}
	h0, a0, h1, a1 = h0&open, a0&open, h1&open, a1&open

	removed, record, grant := h0, h1, h1 // a despawn, a spawn, or one lifetime replacing another
	if t.old != nil && t.now != nil && t.old.id == t.now.id {
		removed = h0 &^ h1 // lost view
		record = h1 &^ h0  // gained view
		if t.now.revision > t.old.revision {
			record = h1 // every viewer gets the update
		}
		grant = h1&^h0 | h1&(a0^a1) // gained view, promoted or demoted
	}
	for m := removed | record | grant; m != 0; m &= m - 1 {
		bit := m & -m
		d := &deltas[w<<6+bits.TrailingZeros64(m)]
		if removed&bit != 0 {
			d.removed = append(d.removed, t.old.id)
		}
		if record&bit != 0 {
			d.records = append(d.records, t.now.record)
		}
		if grant&bit != 0 {
			access := syncstate.View
			if a1&bit != 0 {
				access = syncstate.Authority
			}
			d.access = append(d.access, syncstate.Grant{ID: t.now.id, Access: access})
		}
	}
}

// baseline is the same question asked of every current entry, for one slot.
func (p *syncPublisher) baseline(s int) syncDelta {
	var d syncDelta
	bit := uint64(1) << (s & 63)
	for _, e := range p.entries {
		if e == nil {
			continue
		}
		hold, auth := e.policy.words(s >> 6)
		if hold&bit == 0 {
			continue
		}
		access := syncstate.View
		if auth&bit != 0 {
			access = syncstate.Authority
		}
		d.records = append(d.records, e.record)
		d.access = append(d.access, syncstate.Grant{ID: e.id, Access: access})
	}
	return d
}
