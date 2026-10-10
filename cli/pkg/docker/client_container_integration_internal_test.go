//go:build integration

package docker

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// TestGetContainerLogs_E2E_DemultiplexedOutput verifies, end-to-end against a
// real Docker daemon, that GetContainerLogs returns clean readable log text for
// a non-TTY container instead of the raw multiplexed stream with 8-byte binary
// frame headers that the moby daemon produces for non-TTY containers.
//
// Preconditions: a running Docker daemon (DOCKER_HOST honored via client.FromEnv)
// and network access to pull alpine:latest. The test skips itself when either is
// unavailable, so it is safe to run as part of the integration suite.
func TestGetContainerLogs_E2E_DemultiplexedOutput(t *testing.T) {
	c, err := NewClient(&service.Config{}, nil)
	if err != nil {
		t.Skipf("skipping: Docker client unavailable: %v", err)
	}
	defer c.Close()

	dockerCli := c.client

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if _, err := dockerCli.Ping(ctx, client.PingOptions{}); err != nil {
		t.Skipf("skipping: Docker daemon unreachable: %v", err)
	}

	const imageName = "alpine:latest"
	const containerName = "world-engine-test-logs-demux"

	pull, err := dockerCli.ImagePull(ctx, imageName, client.ImagePullOptions{})
	if err != nil {
		t.Skipf("skipping: unable to pull %s (network required): %v", imageName, err)
	}
	defer pull.Close()
	if err := pull.Wait(ctx); err != nil {
		t.Skipf("skipping: image pull failed: %v", err)
	}

	// Best-effort cleanup of any stale container from a previous run, and ensure
	// the created container is removed after the test.
	_, _ = dockerCli.ContainerRemove(ctx, containerName, client.ContainerRemoveOptions{Force: true})
	t.Cleanup(func() {
		_, _ = dockerCli.ContainerRemove(
			context.Background(), containerName, client.ContainerRemoveOptions{Force: true})
	})

	// Non-TTY container (Tty explicitly false) writing to both stdout and stderr,
	// so the daemon returns the multiplexed/framed stream.
	createRes, err := dockerCli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image: imageName,
			Cmd:   []string{"sh", "-c", "echo stdout-line-AAA; echo stderr-line-BBB 1>&2"},
			Tty:   false,
		},
		Name: containerName,
	})
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	if _, err := dockerCli.ContainerStart(ctx, createRes.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	waitRes := dockerCli.ContainerWait(ctx, createRes.ID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})
	select {
	case res := <-waitRes.Result:
		if res.StatusCode != 0 {
			t.Fatalf("container exited with non-zero status %d", res.StatusCode)
		}
	case werr := <-waitRes.Error:
		t.Fatalf("container wait error: %v", werr)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for container to exit: %v", ctx.Err())
	}

	// Exercise the real GetContainerLogs through the public Client API.
	logText, err := c.GetContainerLogs(ctx, containerName, 0, 0)
	if err != nil {
		t.Fatalf("GetContainerLogs returned error: %v", err)
	}

	// The bug: raw io.ReadAll over a non-TTY log stream leaves 8-byte binary frame
	// headers (NUL bytes) interleaved in the output. After demultiplexing, no NUL
	// bytes should be present.
	if bytes.Contains([]byte(logText), []byte{0}) {
		t.Fatalf("output contains binary frame headers (NUL bytes): %q", logText)
	}
	if !strings.Contains(logText, "stdout-line-AAA") {
		t.Fatalf("output missing stdout line: %q", logText)
	}
	if !strings.Contains(logText, "stderr-line-BBB") {
		t.Fatalf("output missing stderr line: %q", logText)
	}

	// Both streams were written to a single buffer, so the daemon's chronological
	// emit order (stdout first, then stderr) must be preserved.
	want := "stdout-line-AAA\nstderr-line-BBB\n"
	if logText != want {
		t.Fatalf("output = %q, want %q", logText, want)
	}

	// Defensive: assert the exact frame-header prefixes that the buggy code would
	// have leaked are absent from the output.
	if bytes.Contains([]byte(logText), []byte{byte(stdcopy.Stdout), 0, 0, 0}) {
		t.Fatalf("output contains a stdout frame header: %q", logText)
	}
	if bytes.Contains([]byte(logText), []byte{byte(stdcopy.Stderr), 0, 0, 0}) {
		t.Fatalf("output contains a stderr frame header: %q", logText)
	}
}
