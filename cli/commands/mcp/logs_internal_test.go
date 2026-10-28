package mcp

import (
	"context"
	"errors"
	"testing"

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
			name:      "follow window ending (canceled) keeps the tail",
			logs:      []string{"already-streamed"},
			streamErr: context.Canceled,
			wantLogs:  "already-streamed\n",
		},
		{
			name:      "follow window ending (deadline) keeps the tail",
			logs:      []string{"tail"},
			streamErr: context.DeadlineExceeded,
			wantLogs:  "tail\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeLogReader{logs: tc.logs, streamErr: tc.streamErr}

			logs, err := collectContainerLogsFrom(context.Background(), fake, "demo-nats", 42)

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
