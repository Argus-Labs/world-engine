package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/local"
	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

// fakeLogReader serves a fixed stream and records what was asked for.
type fakeLogReader struct {
	logs      []string
	streamErr error

	streamed   string
	streamOpts worldstatus.LogsOpts
}

func (f *fakeLogReader) StreamContainerLogs(
	_ context.Context,
	name string,
	opts worldstatus.LogsOpts,
	out chan<- worldstatus.LogLine,
) error {
	defer close(out)
	f.streamed = name
	f.streamOpts = opts
	for _, line := range f.logs {
		out <- worldstatus.LogLine{Source: name, Line: line}
	}
	return f.streamErr
}

func TestCollectContainerLogsFrom(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		logs       []string
		streamErr  error
		ctxOver    bool
		wantLogs   string
		wantErrSub []string
		wantNotFnd bool
	}{
		{
			name:     "lines joined, tail and window forwarded",
			logs:     []string{"line one", "line two"},
			wantLogs: "line one\nline two\n",
		},
		{
			name:       "missing container is an error, not empty output",
			streamErr:  local.ErrContainerNotFound,
			wantErrSub: []string{"demo-nats", "not found", "is the world running"},
			wantNotFnd: true,
		},
		{
			name:       "stream error propagates with the container name",
			streamErr:  errors.New("read logs: i/o timeout"),
			wantErrSub: []string{"logs for demo-nats", "read logs: i/o timeout"},
		},
		{
			// The runtime swallows ctx errors; lines that made it through are kept.
			name:     "budget ending mid-fetch keeps the tail",
			logs:     []string{"tail"},
			ctxOver:  true,
			wantLogs: "tail\n",
		},
		{
			name:       "budget already spent with nothing streamed is an error",
			ctxOver:    true,
			wantErrSub: []string{"logs for demo-nats", "context canceled"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeLogReader{logs: tc.logs, streamErr: tc.streamErr}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.ctxOver {
				cancel()
			}

			logs, err := collectContainerLogsFrom(ctx, fake, "demo-nats", 42)

			assert.Equal(t, "demo-nats", fake.streamed)
			assert.Equal(t, int32(42), fake.streamOpts.TailLines)
			assert.Equal(t, logWindow, fake.streamOpts.FollowWindow)
			assert.True(t, fake.streamOpts.MissingIsError, "one-shot fetch must ask for the not-found error")
			if len(tc.wantErrSub) == 0 {
				require.NoError(t, err)
				assert.Equal(t, tc.wantLogs, logs)
				return
			}
			require.Error(t, err)
			assert.Empty(t, logs)
			for _, sub := range tc.wantErrSub {
				assert.Contains(t, err.Error(), sub)
			}
			assert.Equal(t, tc.wantNotFnd, errors.Is(err, local.ErrContainerNotFound))
		})
	}
}

func TestCollectShardLogs(t *testing.T) {
	t.Parallel()

	notFound := eris.Wrap(local.ErrContainerNotFound, "p-game-2")
	boom := errors.New("daemon unreachable")
	fetchFrom := func(m map[string]error) func(string) (string, error) {
		return func(name string) (string, error) {
			if err, ok := m[name]; ok {
				return "", err
			}
			return name + " log\n", nil
		}
	}
	cases := []struct {
		name       string
		containers []string
		errs       map[string]error
		ctxOver    bool
		want       string
		wantErr    error
	}{
		{
			name:       "single container, no header",
			containers: []string{"p-game"},
			want:       "p-game log\n",
		},
		{
			name:       "several containers get headers",
			containers: []string{"p-game", "p-game-2"},
			want:       "=== p-game ===\np-game log\n=== p-game-2 ===\np-game-2 log\n",
		},
		{
			name:       "a vanished instance is noted, the rest returned",
			containers: []string{"p-game", "p-game-2"},
			errs:       map[string]error{"p-game-2": notFound},
			want:       "=== p-game ===\np-game log\n=== p-game-2 ===\n(container not found; removed since the status check)\n",
		},
		{
			name:       "every instance vanished is an error",
			containers: []string{"p-game", "p-game-2"},
			errs:       map[string]error{"p-game": notFound, "p-game-2": notFound},
			wantErr:    local.ErrContainerNotFound,
		},
		{
			name:       "single vanished instance is an error",
			containers: []string{"p-game-2"},
			errs:       map[string]error{"p-game-2": notFound},
			wantErr:    local.ErrContainerNotFound,
		},
		{
			name:       "other errors fail the call",
			containers: []string{"p-game", "p-game-2"},
			errs:       map[string]error{"p-game-2": boom},
			wantErr:    boom,
		},
		{
			name:       "budget spent before the first fetch is an error",
			containers: []string{"p-game", "p-game-2"},
			ctxOver:    true,
			wantErr:    context.Canceled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.ctxOver {
				cancel()
			}
			got, err := collectShardLogs(ctx, tc.containers, fetchFrom(tc.errs))
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr), "err = %v", err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCollectShardLogs_BudgetSpentMidWayListsTheRest(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetch := func(name string) (string, error) {
		cancel() // the first fetch uses up the budget
		return name + " log\n", nil
	}
	got, err := collectShardLogs(ctx, []string{"p-game", "p-game-2", "p-game-3"}, fetch)
	require.NoError(t, err)
	assert.Equal(t, "=== p-game ===\np-game log\n(skipped, time budget spent: p-game-2, p-game-3)\n", got)
}
