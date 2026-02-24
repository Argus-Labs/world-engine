package cardinal

// "Shield Arena" — a small deterministic game used for DST/system tests.
//
// Flow per tick:
//   1. InitSystem (Init)       — runs once at startup.
//   2. GameSystem (Update)     — spawns players from JoinGame commands (each entity gets
//                                {Player, Health, Shield}). Processes Attack commands: shield
//                                absorbs damage first; overflow carries to HP. Emits PlayerKilled
//                                system event when HP ≤ 0, or AttackFailed event for invalid/dead
//                                targets.
//   3. CleanupSystem (PostUpdate) — receives PlayerKilled events, destroys dead entities, emits
//                                   PlayerEliminated external event, and sends ReportElimination
//                                   to another shard via OtherWorld.SendCommand.
//   4. VerifySystem (PostUpdate)  — asserts invariants: all alive entities have HP > 0 and all
//                                   shielded entities have Shield.Points > 0.
//
// State is purely command-driven (no time-dependent behavior), making it trivially verifiable.

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
)

// -------------------------------------------------------------------------------------------------
// DST Helpers
// -------------------------------------------------------------------------------------------------

// dstIOTracker collects commands, events, and inter-shard commands processed by DST
// systems. Initialized by the test harness before the first tick; systems append to it.
type dstIOTracker struct {
	processed   []CommandContext[Command]
	events      []event.Event
	iscCommands []event.Event
}

func (t *dstIOTracker) recordCommand(ctx CommandContext[Command]) {
	t.processed = append(t.processed, ctx)
}

func (t *dstIOTracker) recordEvent(payload Event) {
	t.events = append(t.events, event.Event{Kind: event.KindDefault, Payload: payload})
}

func (t *dstIOTracker) recordISC(payload Command) {
	t.iscCommands = append(t.iscCommands, event.Event{Kind: event.KindInterShardCommand, Payload: payload})
}

func (t *dstIOTracker) reset() {
	t.processed = t.processed[:0]
	t.events = t.events[:0]
	t.iscCommands = t.iscCommands[:0]
}

// dstTracker is initialized by the DST fixture before the first tick. Systems append to it
// when they consume a command or emit an event. This is test-only infrastructure.
var dstTracker dstIOTracker

// -------------------------------------------------------------------------------------------------
// Shard systems
// -------------------------------------------------------------------------------------------------

var dstStatsWorld = OtherWorld{
	Region:       "us",
	Organization: "test",
	Project:      "stats",
	ShardID:      "0",
}

func dstRegisterShardSystems(world *World) {
	RegisterSystem(world, dstInitSystem, WithHook(Init))
	RegisterSystem(world, dstGameSystem)
	RegisterSystem(world, dstCleanupSystem, WithHook(PostUpdate))
	RegisterSystem(world, dstVerifySystem, WithHook(PostUpdate))
}

// InitSystem (Init) — runs once, logs startup.

type dstInitState struct {
	BaseSystemState
}

func dstInitSystem(s *dstInitState) {
	// s.Logger().Info().Msg("arena ready")
}

// GameSystem (Update) — processes commands, resolves combat.

type dstGameState struct {
	BaseSystemState
	JoinCmd   WithCommand[dstJoinGame]
	AttackCmd WithCommand[dstAttack]
	Shielded  Exact[struct {
		Player Ref[dstPlayer]
		Health Ref[dstHealth]
		Shield Ref[dstShield]
	}]
	AnyPlayer Contains[struct {
		Player Ref[dstPlayer]
		Health Ref[dstHealth]
	}]
	Killed       WithSystemEventEmitter[dstPlayerKilled]
	AttackFailed WithEvent[dstAttackFailed]
}

func dstGameSystem(s *dstGameState) {
	for cmd := range s.JoinCmd.Iter() {
		dstTracker.recordCommand(CommandContext[Command]{Payload: cmd.Payload, Persona: cmd.Persona})
		_, refs := s.Shielded.Create()
		refs.Player.Set(dstPlayer{Nickname: cmd.Payload.Nickname})
		refs.Health.Set(dstHealth{HP: cmd.Payload.HP})
		refs.Shield.Set(dstShield{Points: cmd.Payload.ShieldPoints})
	}

	for cmd := range s.AttackCmd.Iter() {
		dstTracker.recordCommand(CommandContext[Command]{Payload: cmd.Payload, Persona: cmd.Persona})
		id, dmg := cmd.Payload.TargetID, cmd.Payload.Damage

		if refs, err := s.Shielded.GetByID(id); err == nil {
			sh := refs.Shield.Get()
			if left := sh.Points - dmg; left > 0 {
				refs.Shield.Set(dstShield{Points: left})
				continue
			} else {
				refs.Shield.Remove()
				dmg = -left
			}
		}

		refs, err := s.AnyPlayer.GetByID(id)
		if err != nil {
			evt := dstAttackFailed{TargetID: id, Reason: "invalid target"}
			s.AttackFailed.Emit(evt)
			dstTracker.recordEvent(evt)
			continue
		}
		h := refs.Health.Get()
		if h.HP <= 0 {
			evt := dstAttackFailed{TargetID: id, Reason: "already dead"}
			s.AttackFailed.Emit(evt)
			dstTracker.recordEvent(evt)
			continue
		}
		refs.Health.Set(dstHealth{HP: h.HP - dmg})
		if h.HP-dmg <= 0 {
			s.Killed.Emit(dstPlayerKilled{
				TargetID: id,
				Nickname: refs.Player.Get().Nickname,
			})
		}
	}

	s.Logger().Debug().Time("ts", s.Timestamp()).Msg("game tick")
}

// CleanupSystem (PostUpdate) — destroys dead entities, emits events, notifies other shard.

type dstCleanupState struct {
	BaseSystemState
	Kills     WithSystemEventReceiver[dstPlayerKilled]
	AnyPlayer Contains[struct {
		Player Ref[dstPlayer]
	}]
	Eliminated WithEvent[dstPlayerEliminated]
}

func dstCleanupSystem(s *dstCleanupState) {
	for kill := range s.Kills.Iter() {
		s.AnyPlayer.Destroy(kill.TargetID)
		evt := dstPlayerEliminated{Nickname: kill.Nickname, AtTick: s.Tick()}
		s.Eliminated.Emit(evt)
		dstTracker.recordEvent(evt)
		cmd := dstReportElimination{Nickname: kill.Nickname}
		dstStatsWorld.SendCommand(&s.BaseSystemState, cmd)
		dstTracker.recordISC(cmd)
		s.Logger().Info().Str("player", kill.Nickname).Msg("eliminated")
	}
}

// VerifySystem (PostUpdate) — checks invariants every tick.

type dstVerifyState struct {
	BaseSystemState
	AllPlayers Contains[struct {
		Health Ref[dstHealth]
	}]
	ShieldedPlayers Exact[struct {
		Player Ref[dstPlayer]
		Health Ref[dstHealth]
		Shield Ref[dstShield]
	}]
}

func dstVerifySystem(s *dstVerifyState) {
	for _, e := range s.AllPlayers.Iter() {
		h := e.Health.Get()
		assert.That(h.HP > 0, "invariant violation: entity has HP <= 0")
	}
	for _, e := range s.ShieldedPlayers.Iter() {
		sh := e.Shield.Get()
		assert.That(sh.Points > 0, "invariant violation: entity has Shield.Points <= 0")
	}
}

// -------------------------------------------------------------------------------------------------
// Shard systems
// -------------------------------------------------------------------------------------------------

type dstPlayer struct {
	Nickname string
}

func (dstPlayer) Name() string { return "Player" }

type dstHealth struct {
	HP int
}

func (dstHealth) Name() string { return "Health" }

type dstShield struct {
	Points int
}

func (dstShield) Name() string { return "Shield" }

// Commands

type dstJoinGame struct {
	Nickname     string
	HP           int
	ShieldPoints int
}

func (dstJoinGame) Name() string { return "join_game" }

type dstAttack struct {
	TargetID EntityID
	Damage   int
}

func (dstAttack) Name() string { return "attack" }

// System Events

type dstPlayerKilled struct {
	TargetID EntityID
	Nickname string
}

func (dstPlayerKilled) Name() string { return "player_killed" }

// Events

type dstPlayerEliminated struct {
	Nickname string
	AtTick   uint64
}

func (dstPlayerEliminated) Name() string { return "player_eliminated" }

type dstAttackFailed struct {
	TargetID EntityID
	Reason   string
}

func (dstAttackFailed) Name() string { return "attack_failed" }

// Inter-Shard Commands

type dstReportElimination struct {
	Nickname string
}

func (dstReportElimination) Name() string { return "report_elimination" }
