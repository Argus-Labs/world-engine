package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/local"
	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

// *local.Runtime satisfies containerLogReader: the one-shot MCP log path relies
// on StreamContainerLogs plus the container-existence check, and a compile-time
// assertion keeps that contract from drifting if a method signature changes.
var _ containerLogReader = (*local.Runtime)(nil)

// fakeLogReader is a stand-in containerLogReader for unit-testing the one-shot
// log path without a Docker daemon. It records the names seen and serves a
// fixed existence verdict and log stream, so the missing-container case — which
// on a real daemon pumpLogs would swallow as an empty stream — is under the
// test's control.
type fakeLogReader struct {
	exists    bool
	existsErr error
	logs      []string
	streamErr error

	inspected  string
	streamed   string
	streamOpts worldstatus.LogsOpts
}

func (f *fakeLogReader) ContainerExists(_ context.Context, name string) (bool, error) {
	f.inspected = name
	return f.exists, f.existsErr
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

// TestCollectContainerLogsFrom_MissingContainerReturnsError is the primary
// regression guard for the fix: a container that does not exist must surface as
// an error an agent can act on, not the empty success pumpLogs would produce for
// the TUI. The existence check must also short-circuit before the stream, so a
// missing container never reaches StreamContainerLogs.
func TestCollectContainerLogsFrom_MissingContainerReturnsError(t *testing.T) {
	t.Parallel()
	fake := &fakeLogReader{exists: false} // a missing container, no error, no logs.

	logs, err := collectContainerLogsFrom(context.Background(), fake, "demo-nats", 100)

	require.Error(t, err)
	assert.Empty(t, logs, "a missing container must not return logs")
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "is the world running")
	assert.Contains(t, err.Error(), "demo-nats")
	assert.Empty(t, fake.streamed, "the stream must be skipped when the container is missing")
	assert.Equal(t, "demo-nats", fake.inspected, "the existence check must target the requested container")
}

// TestCollectContainerLogsFrom_InspectErrorPropagates verifies an inspection
// failure (Docker unreachable, auth error, …) is returned wrapped, not turned
// into empty success: the one-shot caller must see the infrastructure failure.
func TestCollectContainerLogsFrom_InspectErrorPropagates(t *testing.T) {
	t.Parallel()
	fake := &fakeLogReader{
		existsErr: errors.New("Cannot connect to the Docker daemon"),
	}

	_, err := collectContainerLogsFrom(context.Background(), fake, "demo-nats", 100)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "check container demo-nats")
	assert.Contains(t, err.Error(), "Cannot connect to the Docker daemon")
	assert.Empty(t, fake.streamed, "the stream must be skipped when the existence check fails")
}

// TestCollectContainerLogsFrom_ExistingContainerReturnsLogs confirms no
// regression for the happy path: an existing container's streamed lines come
// back joined with newlines, and the follow-window/tail options are forwarded.
func TestCollectContainerLogsFrom_ExistingContainerReturnsLogs(t *testing.T) {
	t.Parallel()
	fake := &fakeLogReader{exists: true, logs: []string{"line one", "line two"}}

	logs, err := collectContainerLogsFrom(context.Background(), fake, "demo-nats", 42)

	require.NoError(t, err)
	assert.Equal(t, "line one\nline two\n", logs)
	assert.Equal(t, "demo-nats", fake.streamed)
	assert.Equal(t, int32(42), fake.streamOpts.TailLines)
	assert.Equal(t, logWindow, fake.streamOpts.FollowWindow)
}

// TestCollectContainerLogsFrom_StreamErrorPropagates verifies a non-terminal
// stream error (the daemon cut the stream, a demux read failed, …) is returned
// wrapped with the container name, instead of silently truncating the logs.
func TestCollectContainerLogsFrom_StreamErrorPropagates(t *testing.T) {
	t.Parallel()
	fake := &fakeLogReader{exists: true, streamErr: errors.New("read logs: i/o timeout")}

	_, err := collectContainerLogsFrom(context.Background(), fake, "demo-nats", 100)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "logs for demo-nats")
	assert.Contains(t, err.Error(), "read logs: i/o timeout")
}

// TestCollectContainerLogsFrom_ContextCanceledIsSwallowed preserves the
// existing contract that the follow window ending the context (Canceled, or
// DeadlineExceeded from the follow-window timeout) is not a tool error: the tail
// already streamed is returned. This keeps the one-shot path quiet when the
// logWindow expires, as before the fix.
func TestCollectContainerLogsFrom_ContextCanceledIsSwallowed(t *testing.T) {
	t.Parallel()
	fake := &fakeLogReader{exists: true, logs: []string{"already-streamed"}, streamErr: context.Canceled}

	logs, err := collectContainerLogsFrom(context.Background(), fake, "demo-nats", 100)

	require.NoError(t, err, "context.Canceled (the follow window ending) must not be a tool error")
	assert.Equal(t, "already-streamed\n", logs)
}

func TestCollectContainerLogsFrom_DeadlineExceededIsSwallowed(t *testing.T) {
	t.Parallel()
	fake := &fakeLogReader{exists: true, logs: []string{"tail"}, streamErr: context.DeadlineExceeded}

	logs, err := collectContainerLogsFrom(context.Background(), fake, "demo-nats", 100)

	require.NoError(t, err, "context.DeadlineExceeded (the follow window) must not be a tool error")
	assert.Equal(t, "tail\n", logs)
}

// TestCollectContainerLogs_UnreachableDockerReturnsError wires the real
// *local.Runtime behind collectContainerLogs against a dead Docker socket and
// confirms the one-shot path returns an error rather than an empty success —
// the glue (reader, lock, ContainerExists on the real runtime, error
// propagation) all works end to end. Docker is pointed at a socket that cannot
// exist so the lookup cannot accidentally succeed against a world a developer
// happens to have running.
func TestCollectContainerLogs_UnreachableDockerReturnsError(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(t.TempDir(), "nonexistent.sock"))

	_, err := collectContainerLogs(context.Background(), "mcp-log-test", "mcp-log-test-nats", 100)

	require.Error(t, err, "an unreachable daemon must surface as an error, not empty success")
}

// TestGetNatsLogsHandler_UnreachableDockerReturnsError confirms the handler
// surfaces the error (so mcp-go sets isError: true on the result) instead of
// returning the pre-fix {"component":"nats","logs":""} success an agent would
// read as "NATS had no logs". Uses the same dead-socket trick as above.
func TestGetNatsLogsHandler_UnreachableDockerReturnsError(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(t.TempDir(), "nonexistent.sock"))

	out, err := getNatsLogsHandler(context.Background(), mcp.CallToolRequest{}, GetNatsLogsInput{
		Project: "mcp-log-test",
	})

	require.Error(t, err, "missing/unreachable NATS must be a tool error, not empty success")
	assert.Empty(t, out.Logs, "no logs may be returned when the fetch failed")
}

// TestCollectContainerLogsFrom_FollowWindowBoundedByLogWindow pins the constant
// the one-shot path uses so an accidental change to the follow window (which
// controls how long an MCP call sits on a live container) is caught here.
func TestCollectContainerLogsFrom_FollowWindowBoundedByLogWindow(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 3*time.Second, logWindow, "logWindow must stay short so one-shot MCP fetches return promptly")
}
