package cardinal

import (
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/kernel"
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
// on the tick, against current game state. No connection receives sync before it is admitted,
// and access lists (sync.Viewers, sync.Authorities) only narrow it: authority never bypasses
// admission. Call before StartGame. RegisterComponent remains the only roster of game components.
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
	slices.SortFunc(contract.Components, func(a, b syncstate.Component) int { return strings.Compare(a.Name, b.Name) })
	return contract
}

// AuthorityOf yields, in ascending entity ID order, the synced entities the player has authority
// over, named in sync.Authorities directly or through a group. It reads the world as written
// now, so writes earlier in the same tick count; it does not wait for the tick to publish them.
// It yields nothing for a player EnableSync does not admit. An entity destroyed or replaced while
// the caller consumes the sequence is skipped. Use it for player-level commands that act on
// whatever the player controls. Commands that embed sync.Target are still judged by what the
// server last published.
func (w *World) AuthorityOf(player string) iter.Seq[Entity] {
	return func(yield func(Entity) bool) {
		if !w.sync.enabled() || !w.sync.admit(player) {
			return
		}
		for _, id := range liveAuthority(w, player) {
			e := w.Entity(EntityID(id.Entity))
			if !e.Alive() || e.SyncID() != id {
				continue
			}
			if !yield(e) {
				return
			}
		}
	}
}

func (w *World) ensureSync() *syncPublisher {
	if w.sync != nil {
		return w.sync
	}
	p := &syncPublisher{
		components:   map[ecs.ComponentID]syncstate.Component{},
		presence:     map[EntityID]map[ecs.ComponentID]uint64{},
		groupHolders: map[EntityID]struct{}{},
		groups:       map[string]*syncGroup{},
		policies:     map[string]*syncPolicy{},
		retired:      map[syncstate.ID]*syncEntry{},
		roster:       syncRoster{slotOf: map[string]int{}},
	}
	p.marker = registerBuiltin[syncstate.Sync](w)
	p.group = registerBuiltin[syncstate.Group](w)
	p.viewers = registerBuiltin[syncstate.Viewers](w)
	p.authorities = registerBuiltin[syncstate.Authorities](w)
	w.sync = p
	w.world.Observe(p.observe)
	w.hookSync()
	w.events.RegisterHandler(event.KindSync, w.dispatchSync)
	return p
}

func registerBuiltin[T ecs.Component](w *World) ecs.ComponentID {
	id, err := w.world.RegisterComponent[T]()
	if err != nil {
		panic(err)
	}
	return id
}

func (w *World) registerSync(t reflect.Type, name string, cid ecs.ComponentID) {
	c, exposed, err := syncstate.Describe(t, name)
	if err != nil {
		panic(err)
	}
	if exposed {
		p := w.ensureSync()
		p.components[cid] = c
		p.names, p.index, p.fingerprint = nil, nil, ""
	}
}

// captureSync is a publication barrier before every event and at the end of a tick. It queues
// the transitions of dirty entities as one log item, in order with the tick's events.
func (w *World) captureSync(final bool) {
	if !w.sync.enabled() {
		return
	}
	if commit := w.sync.capture(w, final); len(commit) > 0 {
		w.events.Enqueue(event.Event{Kind: event.KindSync, Payload: syncCommit(commit)})
	}
}

// stampedEvent is an event that names one entity lifetime through an embedded sync.Event.
type stampedEvent interface {
	SyncEntity() uint32
	SyncIdentity(entity uint32) syncstate.ID
}

// stampSync stamps an event about an entity with that entity's current lifetime, unless the
// sender already captured one with sync.For. payload points at the event.
func (w *World) stampSync(payload any) {
	if w.sync == nil {
		return
	}
	if stamped, ok := payload.(interface {
		SyncEntity() uint32
		SetSyncIdentity(syncstate.ID)
	}); ok {
		stamped.SetSyncIdentity(w.Entity(EntityID(stamped.SyncEntity())).SyncID())
	}
}

// syncAudience returns who may receive an event at dispatch, or nil for the default delivery;
// payload points at the event. It captures the fallback an event about an entity is judged by if
// the lifetime has no entry at dispatch: the live lists if the lifetime is synced now, else its
// last published entry, kept until the end of the tick it ended in. It reads access lists and no
// component state.
func (w *World) syncAudience(payload any) *syncAudience {
	p := w.sync
	if !p.enabled() {
		return nil
	}
	_, occurrence := payload.(interface{ SyncOccurrence() })
	audience := &syncAudience{publisher: p, occurrence: occurrence}
	stamped, ok := payload.(stampedEvent)
	if !ok {
		if occurrence {
			return audience
		}
		return nil
	}
	audience.id, audience.stamped = stamped.SyncIdentity(stamped.SyncEntity()), true
	n := EntityID(audience.id.Entity)
	p.refreshGroups(w)
	last := p.entry(n)
	switch live, synced := p.lifetime(w, n); {
	case synced && live == audience.id:
		audience.fallback = p.policyOf(readLists(w, n), last)
	case last != nil && last.id == audience.id:
		audience.fallback = last.policy
	case p.retired[audience.id] != nil: // destroyed this tick, and already captured
		audience.fallback = p.retired[audience.id].policy
	case w.Entity(n).Alive() && w.Entity(n).SyncID() == audience.id:
		audience.ungated = true // alive, and never synced as this lifetime: not about synced state
	}
	return audience
}

// targeted is a command that embeds sync.Target.
type targeted interface {
	SyncTarget() syncstate.ID
	SyncCommandID() uint64
}

// How long, in the tick's server time, and how many per player, judged command IDs are
// remembered. A client that retries a command for longer than this may have it applied twice.
const (
	commandMemoryAge       = 30 * time.Second
	commandMemoryPerPlayer = 256
)

// judgedCommand is the verdict on one command ID, "" if it was admitted.
type judgedCommand struct {
	id      uint64
	at      time.Time // the timestamp of the tick that judged it
	verdict syncstate.Reason
}

// commandMemory holds, per player, the judged command IDs oldest first. It lives on the tick and
// is not persisted. A player with nothing remembered has no entry.
type commandMemory map[string][]judgedCommand

// recall returns the verdict a player's command ID got, if it is remembered. ID 0 never is.
func (m commandMemory) recall(player string, id uint64) (syncstate.Reason, bool) {
	if id == 0 {
		return "", false
	}
	for _, c := range m[player] {
		if c.id == id {
			return c.verdict, true
		}
	}
	return "", false
}

// remember records a verdict, dropping the player's oldest past commandMemoryPerPlayer.
func (m commandMemory) remember(player string, c judgedCommand) {
	if c.id == 0 {
		return
	}
	m[player] = append(m[player], c)
	if over := len(m[player]) - commandMemoryPerPlayer; over > 0 {
		m[player] = slices.Delete(m[player], 0, over)
	}
}

// forget drops every verdict commandMemoryAge old or older at now, and every player left empty.
func (m commandMemory) forget(now time.Time) {
	for player, judged := range m {
		young := slices.IndexFunc(judged, func(c judgedCommand) bool { return now.Sub(c.at) < commandMemoryAge })
		switch young {
		case -1:
			delete(m, player)
		case 0:
		default:
			m[player] = slices.Delete(judged, 0, young)
		}
	}
}

// hookSync registers sync's hooks with the tick, once. A world that registers a command judges
// its commands; the hooks that publish do nothing until EnableSync.
func (w *World) hookSync() {
	if w.hooks != nil {
		return
	}
	w.hooks = kernel.New(w.world)
	w.hooks.Register(kernel.Hooks{
		Name:   "sync",
		Begin:  w.beginJudging,
		Admit:  w.judgeCommand,
		Emit:   w.emitSync,
		Commit: w.commitSync,
		Settle: w.settleSync,
	})
}

// beginJudging forgets, before the tick's first command is judged, the verdicts that have aged
// out and what EnableSync's predicate said about last tick's senders.
func (w *World) beginJudging() {
	if w.judged == nil {
		w.judged = commandMemory{}
	}
	if w.admitted == nil {
		w.admitted = map[string]bool{}
	}
	w.judged.forget(w.Timestamp())
	clear(w.admitted)
}

// judgeCommand rejects a command its sender may not send, before any handler sees it. A target is
// judged by what the server last published. A player's repeat of a remembered command ID is not
// judged again: it gets the first verdict, and never reaches a handler.
func (w *World) judgeCommand(cmd command.Command) kernel.Verdict {
	var id uint64
	if t, ok := cmd.Payload.(targeted); ok {
		id = t.SyncCommandID()
	}
	var reason syncstate.Reason
	repeat := false
	player, isPlayer := cmd.Sender.Player() // only players' IDs are remembered
	if isPlayer {
		reason, repeat = w.judged.recall(player, id)
	}
	if !repeat {
		reason = w.commandVerdict(cmd)
		if isPlayer {
			w.judged.remember(player, judgedCommand{id: id, at: w.Timestamp(), verdict: reason})
		}
	}
	switch {
	case reason != "":
		return kernel.Verdict{Outcome: kernel.Rejected, Reason: string(reason), CommandID: id}
	case repeat:
		return kernel.Verdict{Outcome: kernel.Dropped}
	}
	return kernel.Verdict{}
}

// emitSync is sync's part in sending an event. It stamps the event, narrows a client event to
// the readers who may see it, and publishes the changes made before it, so they arrive first.
func (w *World) emitSync(e *kernel.Emit) {
	w.stampSync(e.Payload)
	if e.Kind != kernel.SystemEvent {
		// Before the capture below can drop a destroyed entity's last entry.
		if audience := w.syncAudience(e.Payload); audience != nil {
			e.Narrow(audience)
		}
	}
	w.captureSync(false)
}

// commitSync publishes what the tick's systems left behind: the last capture, then an open or a
// close for every reader whose stream must change.
func (w *World) commitSync() {
	w.captureSync(true)
	w.reconcileSync()
}

// commandVerdict judges a command no earlier verdict covers; "" admits it.
func (w *World) commandVerdict(cmd command.Command) syncstate.Reason {
	player, isPlayer := cmd.Sender.Player()
	switch w.commandSenders[cmd.Name] {
	case syncstate.FromPlayers:
		if !isPlayer {
			return syncstate.PlayerOnly
		}
	case syncstate.FromShards:
		if isPlayer {
			return syncstate.ShardOnly
		}
	}
	target, ok := cmd.Payload.(targeted)
	if !ok {
		return ""
	}
	p := w.sync
	if !p.enabled() || !isPlayer {
		return syncstate.StaleTarget // only an admitted player can hold authority
	}
	allowed, known := w.admitted[player]
	if !known {
		allowed = p.admit(player)
		w.admitted[player] = allowed
	}
	return p.judge(player, allowed, target.SyncTarget())
}
