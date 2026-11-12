package sync

import "github.com/argus-labs/world-engine/pkg/immutable"

// Group is a named set of players. Put it on an entity of its own and update it in place: one
// membership change applies to every entity whose Viewers or Authorities name the group. Groups
// are ordinary world state, snapshotted and restored with it, and are never sent to clients.
// A name that matches no group has no members. Two groups with the same name are a bug: the
// engine logs an error and treats the name as having no members.
type Group struct {
	GroupName string
	Players   immutable.Slice[string]
}

func (Group) Name() string { return "sync_group" }

// Viewers narrows who may read a synced entity to the listed players, the members of the listed
// groups, and its Authorities. Without the component every admitted player may read the entity.
// An empty Viewers leaves it visible to its Authorities only.
type Viewers struct {
	Players immutable.Slice[string]
	Groups  immutable.Slice[string]
}

func (Viewers) Name() string { return "sync_viewers" }

// Authorities lists who may send commands that target a synced entity, named the same way as
// Viewers. They can always read it too. Without the component nobody can target the entity.
type Authorities struct {
	Players immutable.Slice[string]
	Groups  immutable.Slice[string]
}

func (Authorities) Name() string { return "sync_authorities" }

// Target makes a command act on one entity lifetime. Embed it in the command. Cardinal admits the
// command only when the sender has authority over that lifetime as the server last published it,
// and the handler reads the verified entity through CommandContext.Target. Clients fill it from
// the synced view they act on.
type Target struct {
	TargetSession     string
	TargetEntity      uint32
	TargetIncarnation uint64
}

// SyncTarget returns the lifetime the command targets.
func (t Target) SyncTarget() ID {
	return ID{Session: t.TargetSession, Entity: t.TargetEntity, Incarnation: t.TargetIncarnation}
}

// TargetOf returns a Target for one entity lifetime.
func TargetOf(id ID) Target {
	return Target{TargetSession: id.Session, TargetEntity: id.Entity, TargetIncarnation: id.Incarnation}
}

// Senders declares who may send a command. Pass it to RegisterCommand. A command registered
// without one accepts both kinds of sender.
type Senders uint8

const (
	FromPlayers Senders = 1 << iota // Only clients, as authenticated players
	FromShards                      // Only other shards, through SendToShard
)

// Reason says why Cardinal rejected a command before any handler saw it.
type Reason string

const (
	PlayerOnly  Reason = "player_only"  // A shard sent a command registered FromPlayers
	ShardOnly   Reason = "shard_only"   // A player sent a command registered FromShards
	StaleTarget Reason = "stale_target" // The target is gone, another lifetime, or not visible to the sender
	NoAuthority Reason = "no_authority" // The sender can view the target but has no authority over it
)

// CommandRejected is the event Cardinal sends to a player whose command it rejected. Game-rule
// rejections, such as cooldowns, stay game events.
type CommandRejected struct {
	Command string
	Reason  Reason
}

func (CommandRejected) Name() string { return "command_rejected" }
