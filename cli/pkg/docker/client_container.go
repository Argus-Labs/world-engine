package docker

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
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

// clampTailLines bounds a requested history: 0 or less means the default, and
// anything larger than maxLogTailLines is capped.
func clampTailLines(tail int) int {
	if tail <= 0 || tail > maxLogTailLines {
		return maxLogTailLines
	}
	return tail
}

// maxLogLineBytes caps one streamed log line; longer lines end the scan.
const maxLogLineBytes = 1024 * 1024

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

// removeContainer stops and removes a container; with keepVolumes false its named
// volumes go too. A missing container is a no-op.
func (c *Client) removeContainer(ctx context.Context, name string, keepVolumes bool) error {
	res, err := c.client.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		return eris.Wrapf(err, "inspect container %s", name)
	}
	if _, err := c.client.ContainerStop(ctx, name, client.ContainerStopOptions{Signal: "SIGINT"}); err != nil {
		return eris.Wrapf(err, "stop container %s", name)
	}
	if _, err := c.client.ContainerRemove(ctx, name, client.ContainerRemoveOptions{}); err != nil {
		return eris.Wrapf(err, "remove container %s", name)
	}
	if keepVolumes {
		return nil
	}
	for _, m := range res.Container.Mounts {
		if m.Type != mount.TypeVolume || m.Name == "" {
			continue
		}
		if _, err := c.client.VolumeRemove(
			ctx,
			m.Name,
			client.VolumeRemoveOptions{Force: true},
		); err != nil &&
			!cerrdefs.IsNotFound(err) {
			return eris.Wrapf(err, "remove volume %s", m.Name)
		}
	}
	return nil
}

// InspectContainer inspects a Docker container by name and returns its inspection data.
func (c *Client) InspectContainer(ctx context.Context, containerName string) (container.InspectResponse, error) {
	inspect, err := c.client.ContainerInspect(ctx, containerName, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, eris.Wrapf(err, "failed to inspect container %s", containerName)
	}
	return inspect.Container, nil
}

// CardinalHostPort returns the 127.0.0.1 port bound to a shard container's :8080,
// so callers never depend on the index formula.
func (c *Client) CardinalHostPort(ctx context.Context, containerName string) (int, error) {
	inspect, err := c.InspectContainer(ctx, containerName)
	if err != nil {
		return 0, err
	}
	if inspect.NetworkSettings == nil || inspect.NetworkSettings.Ports == nil {
		return 0, eris.Errorf("container %s has no network port bindings", containerName)
	}
	debugPort := network.MustParsePort(strconv.Itoa(service.CardinalPort) + "/tcp")
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

	// Bound the volume streamed back to an agent, same rule as the follow path.
	opts.Tail = strconv.Itoa(clampTailLines(tailLines))

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

	// Limit total bytes read to protect against OOM from extremely long log lines.
	limitedReader := io.LimitReader(reader, maxLogBytes)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", eris.Wrapf(err, "failed to read logs for container %s", containerName)
	}

	return string(data), nil
}

// startContainer creates the container if missing, then starts it.
func (c *Client) startContainer(ctx context.Context, svc service.Service) error {
	exist, err := c.containerExists(ctx, svc.Name)
	if err != nil {
		return err
	}
	if !exist {
		_, err := c.client.ContainerCreate(ctx, client.ContainerCreateOptions{
			Config:           &svc.Config,
			HostConfig:       &svc.HostConfig,
			NetworkingConfig: &svc.NetworkingConfig,
			Platform:         &svc.Platform,
			Name:             svc.Name,
		})
		if err != nil {
			return eris.Wrapf(err, "create container %s", svc.Name)
		}
	}
	if _, err := c.client.ContainerStart(ctx, svc.Name, client.ContainerStartOptions{}); err != nil {
		return eris.Wrapf(err, "start container %s", svc.Name)
	}
	return nil
}

// stopContainer sends SIGINT and waits for exit; a missing container is a no-op.
func (c *Client) stopContainer(ctx context.Context, name string) error {
	exist, err := c.containerExists(ctx, name)
	if err != nil || !exist {
		return err
	}
	if _, err := c.client.ContainerStop(ctx, name, client.ContainerStopOptions{Signal: "SIGINT"}); err != nil {
		return eris.Wrapf(err, "stop container %s", name)
	}
	return nil
}

// LogEntry is one demuxed container log line.
type LogEntry struct {
	Container string
	Timestamp time.Time // zero when Docker gave none
	Line      string
}

// StreamContainerLogs tails a container's stdout+stderr into out, following live
// output when follow is set, until the stream ends or ctx is cancelled.
func (c *Client) StreamContainerLogs(
	ctx context.Context,
	name string,
	tail int,
	follow bool,
	out chan<- LogEntry,
) error {
	rc, err := c.client.ContainerLogs(ctx, name, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Timestamps: true, Follow: follow,
		Tail: strconv.Itoa(clampTailLines(tail)),
	})
	if err != nil {
		return eris.Wrapf(err, "logs for %s", name)
	}
	defer rc.Close()

	pr, pw := io.Pipe()
	// Closing rc does not unblock a demux write already parked in pw.Write, so the
	// reader end must be closed too or the goroutine and the pipe leak per tail.
	defer pr.Close()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, rc)
		_ = pw.CloseWithError(err)
	}()
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64*1024), maxLogLineBytes)
	for sc.Scan() {
		entry := LogEntry{Container: name, Line: sc.Text()}
		if ts, rest, ok := strings.Cut(entry.Line, " "); ok {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				entry.Timestamp, entry.Line = t, rest
			}
		}
		select {
		case out <- entry:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return eris.Wrapf(err, "read logs for %s", name)
	}
	return ctx.Err()
}
