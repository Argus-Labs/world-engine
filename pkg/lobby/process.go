package lobby

import (
	"context"
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/pkg/lobby/types"
	"github.com/argus-labs/world-engine/pkg/micro"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// processCommands handles all commands in the tick.
func (l *lobby) processCommands(commands []micro.Command, now time.Time) error {
	for _, cmd := range commands {
		cmdName := cmd.Command.Body.Name

		var err error
		switch cmdName {
		// Party commands
		case "create-party":
			err = l.processCreateParty(cmd, now)
		case "join-party":
			err = l.processJoinParty(cmd)
		case "leave-party":
			err = l.processLeaveParty(cmd)
		case "kick-from-party":
			err = l.processKickFromParty(cmd)
		case "disband-party":
			err = l.processDisbandParty(cmd)
		case "set-party-leader":
			err = l.processSetPartyLeader(cmd)
		case "set-party-open":
			err = l.processSetPartyOpen(cmd)

		// Lobby commands
		case "create-lobby":
			err = l.processCreateLobby(cmd, now)
		case "join-lobby":
			err = l.processJoinLobby(cmd)
		case "leave-lobby":
			err = l.processLeaveLobby(cmd)
		case "kick-from-lobby":
			err = l.processKickFromLobby(cmd)
		case "close-lobby":
			err = l.processCloseLobby(cmd)

		// Ready/Match lifecycle
		case "set-ready":
			err = l.processSetReady(cmd)
		case "unset-ready":
			err = l.processUnsetReady(cmd)
		case "start-match":
			err = l.processStartMatch(cmd, now)
		case "end-match":
			err = l.processEndMatch(cmd, now)

		// Internal commands (from Game Shard)
		case "heartbeat":
			err = l.processHeartbeat(cmd, now)
		case "set-player-status":
			err = l.processSetPlayerStatus(cmd)
		// Note: request-backfill and cancel-backfill are handled via service endpoints

		default:
			l.tel.Logger.Warn().Str("command", cmdName).Msg("Unknown command")
		}

		if err != nil {
			l.tel.Logger.Error().Err(err).Str("command", cmdName).Msg("Failed to process command")
			// Continue processing other commands
		}
	}
	return nil
}

// Party command processors

func (l *lobby) processCreateParty(cmd micro.Command, now time.Time) error {
	payload, ok := cmd.Command.Body.Payload.(CreatePartyCommand)
	if !ok {
		return eris.New("invalid payload type for create-party command")
	}

	maxSize := payload.MaxSize
	if maxSize <= 0 {
		maxSize = 5 // Default max party size
	}

	party, err := l.parties.Create(payload.PlayerID, payload.IsOpen, maxSize, now)
	if err != nil {
		return eris.Wrap(err, "failed to create party")
	}

	l.tel.Logger.Info().
		Str("party_id", party.ID).
		Str("leader_id", party.LeaderID).
		Bool("is_open", party.IsOpen).
		Msg("Party created")

	return nil
}

func (l *lobby) processJoinParty(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(JoinPartyCommand)
	if !ok {
		return eris.New("invalid payload type for join-party command")
	}

	if err := l.parties.AddMember(payload.PartyID, payload.PlayerID); err != nil {
		return eris.Wrap(err, "failed to join party")
	}

	l.tel.Logger.Info().
		Str("party_id", payload.PartyID).
		Str("player_id", payload.PlayerID).
		Msg("Player joined party")

	return nil
}

func (l *lobby) processLeaveParty(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(LeavePartyCommand)
	if !ok {
		return eris.New("invalid payload type for leave-party command")
	}

	// If party is in a lobby, leave the lobby first
	party, ok := l.parties.Get(payload.PartyID)
	if ok && party.InLobby() {
		// If this player is the only one in the party, leave lobby
		if len(party.Members) == 1 {
			if _, err := l.lobbies.RemoveParty(party.LobbyID, payload.PartyID); err != nil {
				l.tel.Logger.Warn().Err(err).Msg("Failed to remove party from lobby on leave")
			}
		}
	}

	disbanded, err := l.parties.RemoveMember(payload.PartyID, payload.PlayerID)
	if err != nil {
		return eris.Wrap(err, "failed to leave party")
	}

	if disbanded {
		l.tel.Logger.Info().
			Str("party_id", payload.PartyID).
			Msg("Party disbanded (last member left)")
	} else {
		l.tel.Logger.Info().
			Str("party_id", payload.PartyID).
			Str("player_id", payload.PlayerID).
			Msg("Player left party")
	}

	return nil
}

func (l *lobby) processKickFromParty(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(KickFromPartyCommand)
	if !ok {
		return eris.New("invalid payload type for kick-from-party command")
	}

	party, ok := l.parties.Get(payload.PartyID)
	if !ok {
		return eris.Errorf("party %s not found", payload.PartyID)
	}

	if !party.IsLeader(payload.LeaderID) {
		return eris.Errorf("player %s is not the party leader", payload.LeaderID)
	}

	if payload.LeaderID == payload.TargetPlayerID {
		return eris.New("cannot kick yourself")
	}

	if _, err := l.parties.RemoveMember(payload.PartyID, payload.TargetPlayerID); err != nil {
		return eris.Wrap(err, "failed to kick from party")
	}

	l.tel.Logger.Info().
		Str("party_id", payload.PartyID).
		Str("kicked_player", payload.TargetPlayerID).
		Str("by_leader", payload.LeaderID).
		Msg("Player kicked from party")

	return nil
}

func (l *lobby) processDisbandParty(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(DisbandPartyCommand)
	if !ok {
		return eris.New("invalid payload type for disband-party command")
	}

	party, ok := l.parties.Get(payload.PartyID)
	if !ok {
		return eris.Errorf("party %s not found", payload.PartyID)
	}

	if !party.IsLeader(payload.LeaderID) {
		return eris.Errorf("player %s is not the party leader", payload.LeaderID)
	}

	// If party is in a lobby, leave it first
	if party.InLobby() {
		if _, err := l.lobbies.RemoveParty(party.LobbyID, payload.PartyID); err != nil {
			l.tel.Logger.Warn().Err(err).Msg("Failed to remove party from lobby on disband")
		}
	}

	if !l.parties.Delete(payload.PartyID) {
		return eris.Errorf("failed to delete party %s", payload.PartyID)
	}

	l.tel.Logger.Info().
		Str("party_id", payload.PartyID).
		Str("by_leader", payload.LeaderID).
		Msg("Party disbanded")

	return nil
}

func (l *lobby) processSetPartyLeader(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(SetPartyLeaderCommand)
	if !ok {
		return eris.New("invalid payload type for set-party-leader command")
	}

	party, ok := l.parties.Get(payload.PartyID)
	if !ok {
		return eris.Errorf("party %s not found", payload.PartyID)
	}

	if !party.IsLeader(payload.CurrentLeaderID) {
		return eris.Errorf("player %s is not the party leader", payload.CurrentLeaderID)
	}

	if err := l.parties.SetLeader(payload.PartyID, payload.NewLeaderID); err != nil {
		return eris.Wrap(err, "failed to set party leader")
	}

	l.tel.Logger.Info().
		Str("party_id", payload.PartyID).
		Str("new_leader", payload.NewLeaderID).
		Msg("Party leader changed")

	return nil
}

func (l *lobby) processSetPartyOpen(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(SetPartyOpenCommand)
	if !ok {
		return eris.New("invalid payload type for set-party-open command")
	}

	party, ok := l.parties.Get(payload.PartyID)
	if !ok {
		return eris.Errorf("party %s not found", payload.PartyID)
	}

	if !party.IsLeader(payload.LeaderID) {
		return eris.Errorf("player %s is not the party leader", payload.LeaderID)
	}

	if err := l.parties.SetOpen(payload.PartyID, payload.IsOpen); err != nil {
		return eris.Wrap(err, "failed to set party open status")
	}

	l.tel.Logger.Info().
		Str("party_id", payload.PartyID).
		Bool("is_open", payload.IsOpen).
		Msg("Party open status changed")

	return nil
}

// Lobby command processors

func (l *lobby) processCreateLobby(cmd micro.Command, now time.Time) error {
	payload, ok := cmd.Command.Body.Payload.(CreateLobbyCommand)
	if !ok {
		return eris.New("invalid payload type for create-lobby command")
	}

	// Verify party exists
	party, ok := l.parties.Get(payload.PartyID)
	if !ok {
		return eris.Errorf("party %s not found", payload.PartyID)
	}

	// Check if party is already in a lobby
	if party.InLobby() {
		return eris.Errorf("party %s is already in a lobby", payload.PartyID)
	}

	lobby, err := l.lobbies.Create(payload.PartyID, payload.MinPlayers, payload.MaxPlayers, payload.Config, now)
	if err != nil {
		return eris.Wrap(err, "failed to create lobby")
	}

	// Update party's lobby reference
	if err := l.parties.SetLobby(payload.PartyID, lobby.MatchID); err != nil {
		l.tel.Logger.Warn().Err(err).Msg("Failed to set lobby on party")
	}

	l.tel.Logger.Info().
		Str("match_id", lobby.MatchID).
		Str("host_party_id", payload.PartyID).
		Int("min_players", payload.MinPlayers).
		Int("max_players", payload.MaxPlayers).
		Msg("Lobby created")

	return nil
}

func (l *lobby) processJoinLobby(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(JoinLobbyCommand)
	if !ok {
		return eris.New("invalid payload type for join-lobby command")
	}

	// Verify party exists
	party, ok := l.parties.Get(payload.PartyID)
	if !ok {
		return eris.Errorf("party %s not found", payload.PartyID)
	}

	// Check if party is already in a lobby
	if party.InLobby() {
		return eris.Errorf("party %s is already in a lobby", payload.PartyID)
	}

	if err := l.lobbies.AddParty(payload.LobbyID, payload.PartyID); err != nil {
		return eris.Wrap(err, "failed to join lobby")
	}

	// Update party's lobby reference
	if err := l.parties.SetLobby(payload.PartyID, payload.LobbyID); err != nil {
		l.tel.Logger.Warn().Err(err).Msg("Failed to set lobby on party")
	}

	l.tel.Logger.Info().
		Str("lobby_id", payload.LobbyID).
		Str("party_id", payload.PartyID).
		Msg("Party joined lobby")

	return nil
}

func (l *lobby) processLeaveLobby(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(LeaveLobbyCommand)
	if !ok {
		return eris.New("invalid payload type for leave-lobby command")
	}

	lobbyClosed, err := l.lobbies.RemoveParty(payload.LobbyID, payload.PartyID)
	if err != nil {
		return eris.Wrap(err, "failed to leave lobby")
	}

	// Clear party's lobby reference
	if err := l.parties.SetLobby(payload.PartyID, ""); err != nil {
		l.tel.Logger.Warn().Err(err).Msg("Failed to clear lobby from party")
	}

	// Clear ready status
	if err := l.parties.SetReady(payload.PartyID, false); err != nil {
		l.tel.Logger.Warn().Err(err).Msg("Failed to clear ready status")
	}

	if lobbyClosed {
		l.tel.Logger.Info().
			Str("lobby_id", payload.LobbyID).
			Msg("Lobby closed (last party left)")
	} else {
		l.tel.Logger.Info().
			Str("lobby_id", payload.LobbyID).
			Str("party_id", payload.PartyID).
			Msg("Party left lobby")

		// Check if any party became unready (revert to waiting if needed)
		l.checkAnyPartyUnready(payload.LobbyID)
	}

	return nil
}

func (l *lobby) processKickFromLobby(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(KickFromLobbyCommand)
	if !ok {
		return eris.New("invalid payload type for kick-from-lobby command")
	}

	lobby, ok := l.lobbies.Get(payload.LobbyID)
	if !ok {
		return eris.Errorf("lobby %s not found", payload.LobbyID)
	}

	if !lobby.IsHost(payload.HostPartyID) {
		return eris.Errorf("party %s is not the lobby host", payload.HostPartyID)
	}

	if payload.HostPartyID == payload.TargetPartyID {
		return eris.New("cannot kick yourself from lobby")
	}

	if _, err := l.lobbies.RemoveParty(payload.LobbyID, payload.TargetPartyID); err != nil {
		return eris.Wrap(err, "failed to kick from lobby")
	}

	// Clear target party's lobby reference and ready status
	if err := l.parties.SetLobby(payload.TargetPartyID, ""); err != nil {
		l.tel.Logger.Warn().Err(err).Msg("Failed to clear lobby from kicked party")
	}
	if err := l.parties.SetReady(payload.TargetPartyID, false); err != nil {
		l.tel.Logger.Warn().Err(err).Msg("Failed to clear ready status from kicked party")
	}

	l.tel.Logger.Info().
		Str("lobby_id", payload.LobbyID).
		Str("kicked_party", payload.TargetPartyID).
		Str("by_host", payload.HostPartyID).
		Msg("Party kicked from lobby")

	// Check if any party became unready (revert to waiting if needed)
	l.checkAnyPartyUnready(payload.LobbyID)

	return nil
}

func (l *lobby) processCloseLobby(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(CloseLobbyCommand)
	if !ok {
		return eris.New("invalid payload type for close-lobby command")
	}

	lobby, ok := l.lobbies.Get(payload.LobbyID)
	if !ok {
		return eris.Errorf("lobby %s not found", payload.LobbyID)
	}

	if !lobby.IsHost(payload.HostPartyID) {
		return eris.Errorf("party %s is not the lobby host", payload.HostPartyID)
	}

	// Clear all parties' lobby references
	for _, partyID := range lobby.Parties {
		if err := l.parties.SetLobby(partyID, ""); err != nil {
			l.tel.Logger.Warn().Err(err).Str("party_id", partyID).Msg("Failed to clear lobby from party")
		}
		if err := l.parties.SetReady(partyID, false); err != nil {
			l.tel.Logger.Warn().Err(err).Str("party_id", partyID).Msg("Failed to clear ready status")
		}
	}

	if !l.lobbies.Delete(payload.LobbyID) {
		return eris.Errorf("failed to delete lobby %s", payload.LobbyID)
	}

	l.tel.Logger.Info().
		Str("lobby_id", payload.LobbyID).
		Str("by_host", payload.HostPartyID).
		Msg("Lobby closed")

	return nil
}

// Ready/Match lifecycle processors

func (l *lobby) processSetReady(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(SetReadyCommand)
	if !ok {
		return eris.New("invalid payload type for set-ready command")
	}

	lobby, ok := l.lobbies.Get(payload.LobbyID)
	if !ok {
		return eris.Errorf("lobby %s not found", payload.LobbyID)
	}

	if !lobby.HasParty(payload.PartyID) {
		return eris.Errorf("party %s is not in lobby %s", payload.PartyID, payload.LobbyID)
	}

	if lobby.State != types.LobbyStateWaiting {
		return eris.Errorf("lobby %s is not in waiting state", payload.LobbyID)
	}

	if err := l.parties.SetReady(payload.PartyID, true); err != nil {
		return eris.Wrap(err, "failed to set ready")
	}

	l.tel.Logger.Debug().
		Str("lobby_id", payload.LobbyID).
		Str("party_id", payload.PartyID).
		Msg("Party set ready")

	// Check if all parties are ready
	l.checkAllPartiesReady(payload.LobbyID)

	return nil
}

func (l *lobby) processUnsetReady(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(UnsetReadyCommand)
	if !ok {
		return eris.New("invalid payload type for unset-ready command")
	}

	lobby, ok := l.lobbies.Get(payload.LobbyID)
	if !ok {
		return eris.Errorf("lobby %s not found", payload.LobbyID)
	}

	if !lobby.HasParty(payload.PartyID) {
		return eris.Errorf("party %s is not in lobby %s", payload.PartyID, payload.LobbyID)
	}

	if lobby.State != types.LobbyStateWaiting && lobby.State != types.LobbyStateReady {
		return eris.Errorf("lobby %s is not in waiting or ready state", payload.LobbyID)
	}

	if err := l.parties.SetReady(payload.PartyID, false); err != nil {
		return eris.Wrap(err, "failed to unset ready")
	}

	l.tel.Logger.Debug().
		Str("lobby_id", payload.LobbyID).
		Str("party_id", payload.PartyID).
		Msg("Party unset ready")

	// Check if lobby needs to revert to waiting
	l.checkAnyPartyUnready(payload.LobbyID)

	return nil
}

func (l *lobby) processStartMatch(cmd micro.Command, now time.Time) error {
	payload, ok := cmd.Command.Body.Payload.(StartMatchCommand)
	if !ok {
		return eris.New("invalid payload type for start-match command")
	}

	lobby, ok := l.lobbies.Get(payload.LobbyID)
	if !ok {
		return eris.Errorf("lobby %s not found", payload.LobbyID)
	}

	if !lobby.IsHost(payload.HostPartyID) {
		return eris.Errorf("party %s is not the lobby host", payload.HostPartyID)
	}

	if !lobby.CanStart() {
		return eris.Errorf("lobby %s cannot start (state: %s)", payload.LobbyID, lobby.State)
	}

	// Transition to in_game
	if err := l.lobbies.SetState(payload.LobbyID, types.LobbyStateInGame); err != nil {
		return eris.Wrap(err, "failed to set lobby state to in_game")
	}

	if err := l.lobbies.SetStartedAt(payload.LobbyID, now); err != nil {
		return eris.Wrap(err, "failed to set started timestamp")
	}

	if err := l.lobbies.UpdateHeartbeat(payload.LobbyID, now); err != nil {
		return eris.Wrap(err, "failed to set initial heartbeat")
	}

	l.tel.Logger.Info().
		Str("lobby_id", payload.LobbyID).
		Str("by_host", payload.HostPartyID).
		Int("party_count", len(lobby.Parties)).
		Msg("Match started")

	return nil
}

func (l *lobby) processEndMatch(cmd micro.Command, now time.Time) error {
	payload, ok := cmd.Command.Body.Payload.(EndMatchCommand)
	if !ok {
		return eris.New("invalid payload type for end-match command")
	}

	lobby, ok := l.lobbies.Get(payload.LobbyID)
	if !ok {
		return eris.Errorf("lobby %s not found", payload.LobbyID)
	}

	if lobby.State != types.LobbyStateInGame {
		return eris.Errorf("lobby %s is not in game", payload.LobbyID)
	}

	// Clear lobby reference from all parties
	for _, partyID := range lobby.Parties {
		if err := l.parties.SetLobby(partyID, ""); err != nil {
			l.tel.Logger.Warn().Err(err).Str("party_id", partyID).Msg("Failed to clear lobby from party")
		}
		if err := l.parties.SetReady(partyID, false); err != nil {
			l.tel.Logger.Warn().Err(err).Str("party_id", partyID).Msg("Failed to clear ready status")
		}
	}

	// Delete the lobby immediately
	l.lobbies.Delete(payload.LobbyID)

	l.tel.Logger.Info().
		Str("lobby_id", payload.LobbyID).
		Int("parties_cleared", len(lobby.Parties)).
		Msg("Match ended, lobby cleaned up")

	return nil
}

// Internal command processors

func (l *lobby) processHeartbeat(cmd micro.Command, now time.Time) error {
	payload, ok := cmd.Command.Body.Payload.(HeartbeatCommand)
	if !ok {
		return eris.New("invalid payload type for heartbeat command")
	}

	if err := l.lobbies.UpdateHeartbeat(payload.LobbyID, now); err != nil {
		return eris.Wrap(err, "failed to update heartbeat")
	}

	l.tel.Logger.Debug().
		Str("lobby_id", payload.LobbyID).
		Msg("Heartbeat received")

	return nil
}

func (l *lobby) processSetPlayerStatus(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(SetPlayerStatusCommand)
	if !ok {
		return eris.New("invalid payload type for set-player-status command")
	}

	lobby, ok := l.lobbies.Get(payload.LobbyID)
	if !ok {
		return eris.Errorf("lobby %s not found", payload.LobbyID)
	}

	// Must be in_game state
	if lobby.State != types.LobbyStateInGame {
		return eris.Errorf("lobby %s is not in game (state: %s)", payload.LobbyID, lobby.State)
	}

	// Verify party is in lobby
	if !lobby.HasParty(payload.PartyID) {
		return eris.Errorf("party %s not in lobby %s", payload.PartyID, payload.LobbyID)
	}

	// Update connection status
	if err := l.lobbies.SetPartyConnected(payload.LobbyID, payload.PartyID, payload.Connected); err != nil {
		return eris.Wrap(err, "failed to update party connection status")
	}

	status := "disconnected"
	if payload.Connected {
		status = "connected"
	}

	l.tel.Logger.Info().
		Str("lobby_id", payload.LobbyID).
		Str("party_id", payload.PartyID).
		Str("status", status).
		Msg("Player status updated")

	return nil
}

// =============================================================================
// Internal Command Processors
// =============================================================================
// These processors handle internal commands queued by service handlers.
// They ensure deterministic state changes by running within Tick().

// processInternalCommands handles all internal commands queued by service handlers.
func (l *lobby) processInternalCommands(now time.Time) error {
	// Process and clear the queue
	commands := l.internalQueue
	l.internalQueue = nil

	for _, cmd := range commands {
		cmdName := cmd.InternalName()
		var err error

		switch c := cmd.(type) {
		case ReceiveMatchInternalCommand:
			err = l.processReceiveMatch(c, now)
		case ReceiveBackfillMatchInternalCommand:
			err = l.processReceiveBackfillMatch(c, now)
		case GameHeartbeatInternalCommand:
			err = l.processGameHeartbeat(c, now)
		case GamePlayerStatusInternalCommand:
			err = l.processGamePlayerStatus(c)
		case GameEndMatchInternalCommand:
			err = l.processGameEndMatch(c)
		default:
			l.tel.Logger.Warn().Str("command", cmdName).Msg("Unknown internal command")
		}

		if err != nil {
			l.tel.Logger.Error().Err(err).Str("command", cmdName).Msg("Failed to process internal command")
			// Continue processing other commands
		}
	}
	return nil
}

// processReceiveMatch handles a match received from Matchmaking Shard.
func (l *lobby) processReceiveMatch(cmd ReceiveMatchInternalCommand, now time.Time) error {
	// First, create parties from match tickets
	for _, team := range cmd.Teams {
		for _, ticket := range team.Tickets {
			_, err := l.parties.CreateFromMatch(ticket.ID, ticket.PlayerIDs, now)
			if err != nil {
				// Party might already exist (e.g., from a previous match attempt)
				l.tel.Logger.Warn().
					Err(err).
					Str("ticket_id", ticket.ID).
					Msg("Failed to create party from match ticket")
			} else {
				l.tel.Logger.Debug().
					Str("ticket_id", ticket.ID).
					Int("player_count", len(ticket.PlayerIDs)).
					Msg("Party created from match ticket")
			}
		}
	}

	// Convert to internal types
	teams := make([]types.LobbyTeam, len(cmd.Teams))
	for i, team := range cmd.Teams {
		partyIDs := make([]string, len(team.Tickets))
		for j, ticket := range team.Tickets {
			partyIDs[j] = ticket.ID
		}
		teams[i] = types.LobbyTeam{
			Name:     team.Name,
			PartyIDs: partyIDs,
		}
	}

	// Convert addresses
	var matchmakingAddr, targetAddr *microv1.ServiceAddress
	if cmd.MatchmakingAddress != nil {
		realm := microv1.ServiceAddress_REALM_WORLD
		if cmd.MatchmakingAddress.Realm == "internal" {
			realm = microv1.ServiceAddress_REALM_INTERNAL
		}
		matchmakingAddr = &microv1.ServiceAddress{
			Region:       cmd.MatchmakingAddress.Region,
			Organization: cmd.MatchmakingAddress.Organization,
			Project:      cmd.MatchmakingAddress.Project,
			ServiceId:    cmd.MatchmakingAddress.ServiceID,
			Realm:        realm,
		}
	}
	if cmd.TargetAddress != nil {
		realm := microv1.ServiceAddress_REALM_WORLD
		if cmd.TargetAddress.Realm == "internal" {
			realm = microv1.ServiceAddress_REALM_INTERNAL
		}
		targetAddr = &microv1.ServiceAddress{
			Region:       cmd.TargetAddress.Region,
			Organization: cmd.TargetAddress.Organization,
			Project:      cmd.TargetAddress.Project,
			ServiceId:    cmd.TargetAddress.ServiceID,
			Realm:        realm,
		}
	}

	// Create lobby
	lobby, err := l.lobbies.CreateFromMatch(
		cmd.MatchID,
		cmd.MatchProfileName,
		teams,
		cmd.Config,
		matchmakingAddr,
		targetAddr,
		now,
	)
	if err != nil {
		return eris.Wrapf(err, "failed to create lobby from match %s", cmd.MatchID)
	}

	// Update party lobby references
	for _, partyID := range lobby.Parties {
		if err := l.parties.SetLobby(partyID, lobby.MatchID); err != nil {
			l.tel.Logger.Warn().Err(err).Str("party_id", partyID).Msg("Failed to set lobby on party")
		}
	}

	l.tel.Logger.Info().
		Str("match_id", cmd.MatchID).
		Str("profile", cmd.MatchProfileName).
		Int("team_count", len(teams)).
		Int("party_count", len(lobby.Parties)).
		Msg("Lobby created from match")

	// Notify Game Shard that the match is ready
	if lobby.TargetAddress != nil && l.service != nil {
		ctx := context.Background()
		if err := l.service.SendStartGame(ctx, lobby); err != nil {
			l.tel.Logger.Error().
				Err(err).
				Str("match_id", cmd.MatchID).
				Msg("Failed to send start-game to Game Shard")
			// Don't return error - lobby is created, just notification failed
		} else {
			// Transition to InGame state after successfully sending start-game
			if err := l.lobbies.SetState(lobby.MatchID, types.LobbyStateInGame); err != nil {
				l.tel.Logger.Warn().
					Err(err).
					Str("match_id", cmd.MatchID).
					Msg("Failed to set lobby state to in_game")
			}
			l.lobbies.SetStartedAt(lobby.MatchID, now)
			l.tel.Logger.Info().
				Str("match_id", cmd.MatchID).
				Msg("Start-game sent to Game Shard, lobby now in_game")
		}
	} else {
		l.tel.Logger.Debug().
			Bool("target_addr_nil", lobby.TargetAddress == nil).
			Bool("service_nil", l.service == nil).
			Str("match_id", cmd.MatchID).
			Msg("Skipping start-game notification (target or service not available)")
	}

	return nil
}

// processReceiveBackfillMatch handles a backfill match received from Matchmaking Shard.
func (l *lobby) processReceiveBackfillMatch(cmd ReceiveBackfillMatchInternalCommand, now time.Time) error {
	lobby, ok := l.lobbies.Get(cmd.MatchID)
	if !ok {
		return eris.Errorf("lobby for match %s not found", cmd.MatchID)
	}

	// Add backfilled parties to the lobby and team
	for _, ticket := range cmd.Tickets {
		// First, create the party from the ticket
		_, err := l.parties.CreateFromMatch(ticket.ID, ticket.PlayerIDs, now)
		if err != nil {
			l.tel.Logger.Warn().
				Err(err).
				Str("ticket_id", ticket.ID).
				Msg("Failed to create backfill party")
		} else {
			l.tel.Logger.Debug().
				Str("ticket_id", ticket.ID).
				Int("player_count", len(ticket.PlayerIDs)).
				Msg("Backfill party created")
		}

		// Add party to lobby
		if err := l.lobbies.AddParty(lobby.MatchID, ticket.ID); err != nil {
			l.tel.Logger.Warn().Err(err).Str("ticket_id", ticket.ID).Msg("Failed to add backfill party to lobby")
			continue
		}

		// Set lobby reference on party
		if err := l.parties.SetLobby(ticket.ID, lobby.MatchID); err != nil {
			l.tel.Logger.Warn().Err(err).Str("ticket_id", ticket.ID).Msg("Failed to set lobby on backfill party")
		}

		// Update team assignments
		for i := range lobby.Teams {
			if lobby.Teams[i].Name == cmd.TeamName {
				lobby.Teams[i].PartyIDs = append(lobby.Teams[i].PartyIDs, ticket.ID)
				break
			}
		}
	}

	l.tel.Logger.Info().
		Str("match_id", cmd.MatchID).
		Str("team_name", cmd.TeamName).
		Int("backfilled_count", len(cmd.Tickets)).
		Msg("Backfill match processed")

	return nil
}

// processGameHeartbeat handles heartbeat from Game Shard.
func (l *lobby) processGameHeartbeat(cmd GameHeartbeatInternalCommand, now time.Time) error {
	lobby, ok := l.lobbies.Get(cmd.MatchID)
	if !ok {
		return eris.Errorf("lobby for match %s not found", cmd.MatchID)
	}

	if lobby.State != types.LobbyStateInGame {
		return eris.Errorf("lobby %s is not in game", cmd.MatchID)
	}

	if err := l.lobbies.UpdateHeartbeat(cmd.MatchID, now); err != nil {
		return eris.Wrap(err, "failed to update heartbeat")
	}

	l.tel.Logger.Debug().
		Str("match_id", cmd.MatchID).
		Msg("Game heartbeat processed")

	return nil
}

// processGamePlayerStatus handles player status from Game Shard.
func (l *lobby) processGamePlayerStatus(cmd GamePlayerStatusInternalCommand) error {
	lobby, ok := l.lobbies.Get(cmd.MatchID)
	if !ok {
		return eris.Errorf("lobby for match %s not found", cmd.MatchID)
	}

	if lobby.State != types.LobbyStateInGame {
		return eris.Errorf("lobby %s is not in game", cmd.MatchID)
	}

	// Find the party containing this player
	partyID := ""
	for _, pid := range lobby.Parties {
		party, exists := l.parties.Get(pid)
		if exists && party.HasMember(cmd.PlayerID) {
			partyID = pid
			break
		}
	}

	if partyID == "" {
		return eris.Errorf("player %s not found in match %s", cmd.PlayerID, cmd.MatchID)
	}

	// Update party connection status
	if err := l.lobbies.SetPartyConnected(cmd.MatchID, partyID, cmd.Connected); err != nil {
		return eris.Wrap(err, "failed to update party connection status")
	}

	status := "disconnected"
	if cmd.Connected {
		status = "connected"
	}

	l.tel.Logger.Info().
		Str("match_id", cmd.MatchID).
		Str("player_id", cmd.PlayerID).
		Str("party_id", partyID).
		Str("status", status).
		Msg("Game player status updated")

	return nil
}

// processGameEndMatch handles end-match from Game Shard.
func (l *lobby) processGameEndMatch(cmd GameEndMatchInternalCommand) error {
	lobby, ok := l.lobbies.Get(cmd.MatchID)
	if !ok {
		return eris.Errorf("lobby for match %s not found", cmd.MatchID)
	}

	if lobby.State != types.LobbyStateInGame {
		return eris.Errorf("lobby %s is not in game", cmd.MatchID)
	}

	// Clear lobby reference from all parties
	for _, partyID := range lobby.Parties {
		if err := l.parties.SetLobby(partyID, ""); err != nil {
			l.tel.Logger.Warn().Err(err).Str("party_id", partyID).Msg("Failed to clear lobby from party")
		}
		if err := l.parties.SetReady(partyID, false); err != nil {
			l.tel.Logger.Warn().Err(err).Str("party_id", partyID).Msg("Failed to clear ready status")
		}
	}

	// Delete the lobby
	l.lobbies.Delete(cmd.MatchID)

	l.tel.Logger.Info().
		Str("match_id", cmd.MatchID).
		Str("result", cmd.Result).
		Int("parties_cleared", len(lobby.Parties)).
		Msg("Game match ended, lobby cleaned up")

	return nil
}
