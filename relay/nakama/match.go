// match.go defines a struct that implements Nakama's runtime.Match interface.
// Matches are used to broadcast Cardinal data to connected clients. A client is 'connected' if they
// are in a Nakama match.
// See https://heroiclabs.com/docs/nakama/client-libraries/index.html for information on using Nakama client
// libraries to join matches and consume broadcasted messages.
package main

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/heroiclabs/nakama-common/runtime"
)

type ReceiptMatch struct{}

var _ runtime.Match = &ReceiptMatch{}

type ReceiptMatchState struct {
	chanID         string
	receiptsToSend receiptChan
}

func (m *ReceiptMatch) MatchInit(_ context.Context, _ runtime.Logger, _ *sql.DB, _ runtime.NakamaModule,
	_ map[string]interface{}) (interface{}, int, string) {
	channelLimit := 100
	state := &ReceiptMatchState{
		chanID:         "singleton-match",
		receiptsToSend: make(receiptChan, channelLimit),
	}
	globalReceiptsDispatcher.subscribe(state.chanID, state.receiptsToSend)
	tickRate := 1 // 1 tick per second = 1 MatchLoop func invocations per second
	label := ""
	return state, tickRate, label
}

func (m *ReceiptMatch) MatchJoin(_ context.Context, logger runtime.Logger, _ *sql.DB, _ runtime.NakamaModule,
	_ runtime.MatchDispatcher, _ int64, stateIface interface{}, _ []runtime.Presence) interface{} {
	state, ok := stateIface.(*ReceiptMatchState)
	if !ok {
		logger.Error("state not a valid lobby state object")
		return nil
	}

	return state
}

func (m *ReceiptMatch) MatchLeave(_ context.Context, logger runtime.Logger, _ *sql.DB, _ runtime.NakamaModule,
	_ runtime.MatchDispatcher, _ int64, stateIface interface{}, _ []runtime.Presence) interface{} {
	state, ok := stateIface.(*ReceiptMatchState)
	if !ok {
		logger.Error("state not a valid lobby state object")
		return nil
	}

	return state
}

const (
	receiptOpCode = 100
)

func (m *ReceiptMatch) MatchLoop(_ context.Context, logger runtime.Logger, _ *sql.DB, _ runtime.NakamaModule,
	dispatcher runtime.MatchDispatcher, _ int64, stateIface interface{}, _ []runtime.MatchData) interface{} {
	state, ok := stateIface.(*ReceiptMatchState)
	if !ok {
		logger.Error("state not a valid lobby state object")
		return nil
	}
	var receiptsToSend []*Receipt

	more := true
	for more {
		select {
		case r := <-state.receiptsToSend:
			receiptsToSend = append(receiptsToSend, r)
		default:
			more = false
		}
	}

	for _, r := range receiptsToSend {
		buf, err := json.Marshal(r)
		if err != nil {
			continue
		}
		err = dispatcher.BroadcastMessage(receiptOpCode, buf, nil, nil, true)
		if err != nil {
			_, _ = logError(logger, "error broadcasting message: %w", err)
		}
	}

	return state
}
func (m *ReceiptMatch) MatchJoinAttempt(_ context.Context, _ runtime.Logger, _ *sql.DB, _ runtime.NakamaModule,
	_ runtime.MatchDispatcher, _ int64, state interface{}, _ runtime.Presence,
	_ map[string]string) (interface{}, bool, string) {
	return state, true, ""
}

func (m *ReceiptMatch) MatchTerminate(_ context.Context, _ runtime.Logger, _ *sql.DB, _ runtime.NakamaModule,
	_ runtime.MatchDispatcher, _ int64, _ interface{}, _ int) interface{} {
	return nil
}

func (m *ReceiptMatch) MatchSignal(_ context.Context, _ runtime.Logger, _ *sql.DB, _ runtime.NakamaModule,
	_ runtime.MatchDispatcher, _ int64, _ interface{}, _ string) (interface{}, string) {
	return nil, ""
}
