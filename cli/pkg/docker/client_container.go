package docker

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// maxLogTailLines is the maximum number of log lines to return from a container,
// preventing excessive log streaming with AI agents.
const maxLogTailLines = 2000

// maxLogSinceSeconds is the maximum time window for log retrieval (24 hours),
// preventing requests for logs from days/weeks ago which could return huge volumes.
const maxLogSinceSeconds = 86400 // 24 hours

// maxLogBytes is the maximum total bytes to read from container logs (10MB),
// protecting against OOM if log lines are extremely long.
const maxLogBytes = 10 * 1024 * 1024

func (c *Client) containerExists(ctx context.Context, containerName string) (bool, error) {
	_, err := c.client.ContainerInspect(ctx, containerName, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil
		}
		return false, eris.Wrapf(err, "Failed to inspect container %s", containerName)
	}

	return true, nil
}

// removeContainerKeepVolume stops and removes a container without deleting its volume.
func (c *Client) removeContainerKeepVolume(ctx context.Context, containerName string) error {
	// Check if the container exists
	exist, err := c.containerExists(ctx, containerName)
	if err != nil {
		return eris.Wrapf(err, "Failed to check if container %s exists", containerName)
	}
	if !exist {
		c.logger.DebugContext(ctx, "container does not exist", "container", containerName)
		return nil
	}

	// Stop the container
	_, err = c.client.ContainerStop(ctx, containerName, client.ContainerStopOptions{
		Signal: "SIGINT",
	})
	if err != nil {
		return eris.Wrapf(err, "Failed to stop container %s", containerName)
	}

	// Remove the container but leave any associated volumes intact
	_, err = c.client.ContainerRemove(ctx, containerName, client.ContainerRemoveOptions{})
	if err != nil {
		return eris.Wrapf(err, "Failed to remove container %s", containerName)
	}

	return nil
}

// readContainerLogs demultiplexes the stdout/stderr stream returned by the
// Docker daemon into a single readable string.
//
// For non-TTY containers (which is the case for every world-cli-managed
// container), the daemon multiplexes stdout and stderr into a single stream
// where each chunk is prefixed with an 8-byte binary header
// (STREAM_TYPE, 0, 0, 0, SIZE1..4). Reading that stream verbatim yields
// garbled output with those headers interleaved between log lines, so it is
// demultiplexed with [stdcopy.StdCopy] before being returned.
//
// stdout and stderr are demultiplexed into separate buffers and concatenated
// (all stdout, then all stderr). The daemon does not guarantee the relative
// ordering of stdout and stderr frames in the multiplexed stream, so grouping
// by stream keeps the returned text deterministic for callers while still
// preserving the within-stream order of each. The total read is capped at
// maxLogBytes of the underlying multiplexed stream to protect against
// unbounded output; a frame truncated by the cap is dropped rather than
// returned partially.
func readContainerLogs(reader io.Reader) (string, error) {
	var outBuf, errBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&outBuf, &errBuf, io.LimitReader(reader, maxLogBytes)); err != nil {
		return "", err
	}
	return outBuf.String() + errBuf.String(), nil
}

// InspectContainer inspects a Docker container by name and returns its inspection data.
func (c *Client) InspectContainer(ctx context.Context, containerName string) (container.InspectResponse, error) {
	inspect, err := c.client.ContainerInspect(ctx, containerName, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, eris.Wrapf(err, "failed to inspect container %s", containerName)
	}
	return inspect.Container, nil
}

// CardinalDebugHostPort returns the host port bound to the Cardinal debug port (8080/tcp)
// for the given container. Use this to connect to the debug API from the host without
// relying on index-based port formulas. Returns an error if the container is not found
// or the port binding is missing.
func (c *Client) CardinalDebugHostPort(ctx context.Context, containerName string) (int, error) {
	inspect, err := c.InspectContainer(ctx, containerName)
	if err != nil {
		return 0, err
	}
	if inspect.NetworkSettings == nil || inspect.NetworkSettings.Ports == nil {
		return 0, eris.Errorf("container %s has no network port bindings", containerName)
	}
	debugPort := network.MustParsePort(strconv.Itoa(service.DefaultCardinalDebugPort) + "/tcp")
	bindings, ok := inspect.NetworkSettings.Ports[debugPort]
	if !ok || len(bindings) == 0 {
		return 0, eris.Errorf("container %s has no host port for %s", containerName, debugPort)
	}
	hostPort, err := strconv.Atoi(bindings[0].HostPort)
	if err != nil {
		return 0, eris.Wrapf(
			err,
			"container %s invalid host port %q for %s",
			containerName,
			bindings[0].HostPort,
			debugPort,
		)
	}
	return hostPort, nil
}

// GetContainerLogs returns logs for the given container. It supports optional tail
// and sinceSeconds filters so callers can limit the volume of logs returned.
// If both tailLines and sinceSeconds are specified, logs are filtered by time first,
// then limited to the specified number of tail lines.
func (c *Client) GetContainerLogs(
	ctx context.Context,
	containerName string,
	tailLines int,
	sinceSeconds int,
) (string, error) {
	if containerName == "" {
		return "", eris.New("container name cannot be empty")
	}

	opts := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: false,
		Follow:     false,
	}

	// Enforce a sane upper bound on log volume. If tailLines is zero or
	// excessively large, cap it to maxLogTailLines to avoid streaming an
	// unbounded amount of data back to the agent.
	if tailLines <= 0 || tailLines > maxLogTailLines {
		tailLines = maxLogTailLines
	}
	opts.Tail = strconv.Itoa(tailLines)

	// Cap sinceSeconds to maxLogSinceSeconds (24 hours) to prevent requests
	// for logs from days/weeks ago which could return huge volumes of data.
	if sinceSeconds > maxLogSinceSeconds {
		sinceSeconds = maxLogSinceSeconds
	}
	if sinceSeconds > 0 {
		sinceTime := time.Now().Add(-time.Duration(sinceSeconds) * time.Second)
		opts.Since = sinceTime.Format(time.RFC3339)
	}

	reader, err := c.client.ContainerLogs(ctx, containerName, opts)
	if err != nil {
		return "", eris.Wrapf(err, "failed to fetch logs for container %s", containerName)
	}
	defer reader.Close()

	logText, err := readContainerLogs(reader)
	if err != nil {
		return "", eris.Wrapf(err, "failed to read logs for container %s", containerName)
	}

	return logText, nil
}
