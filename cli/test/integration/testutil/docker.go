//go:build integration

// Package testutil provides helper functions for integration tests.
package testutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/moby/moby/client"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"

	dockerpkg "github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// ProjectName reads the project a world.toml declares; container names derive from it,
// not from the directory name.
func ProjectName(projectDir string) (string, error) {
	cfg, err := worldtoml.LoadFile(filepath.Join(projectDir, worldtoml.FileName))
	if err != nil {
		return "", err
	}
	return cfg.Project, nil
}

// CardinalShardContainerName derives the Docker container name for one shard
// instance of the project at projectDir.
func CardinalShardContainerName(projectDir, instanceID string) (string, error) {
	project, err := ProjectName(projectDir)
	if err != nil {
		return "", err
	}
	return service.CardinalShardContainerName(project, instanceID), nil
}

// NatsContainerName is the project's NATS container.
func NatsContainerName(projectDir string) (string, error) {
	project, err := ProjectName(projectDir)
	if err != nil {
		return "", err
	}
	return service.NatsContainerName(project), nil
}

// DockerClient wraps the Docker API client for test utilities.
type DockerClient struct {
	cli *client.Client
}

// NewDockerClient creates a new Docker client for tests.
func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerClient{cli: cli}, nil
}

// Close closes the Docker client connection.
func (d *DockerClient) Close() error {
	return d.cli.Close()
}

// ContainerInfo holds information about a container.
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	State   string
	Status  string
	Ports   []string
	Running bool
	Healthy bool
	Project string
}

// ListCardinalContainers returns every container world start created for a
// project: shards, NATS, the project database and [[services]].
func (d *DockerClient) ListCardinalContainers(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := d.cli.ContainerList(ctx, client.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}

	var result []ContainerInfo
	for _, c := range containers.Items {
		// Get clean container name
		var cleanName string
		if len(c.Names) > 0 {
			cleanName = strings.TrimPrefix(c.Names[0], "/")
		}

		// Every container world start creates carries the project label.
		project := c.Labels[service.ProjectLabel]
		if project == "" {
			continue
		}

		info := ContainerInfo{
			ID:      c.ID[:12],
			Name:    cleanName,
			Image:   c.Image,
			State:   string(c.State),
			Status:  c.Status,
			Running: string(c.State) == "running",
			Project: project,
		}

		// Check health status
		if strings.Contains(c.Status, "(healthy)") {
			info.Healthy = true
		}

		// Extract ports
		for _, port := range c.Ports {
			if port.PublicPort > 0 {
				info.Ports = append(info.Ports, strconv.FormatUint(uint64(port.PublicPort), 10))
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// ContainerExists checks if a container with the given name exists.
func (d *DockerClient) ContainerExists(ctx context.Context, name string) (bool, error) {
	containers, err := d.cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("name", name),
	})
	if err != nil {
		return false, err
	}
	return len(containers.Items) > 0, nil
}

// ContainerRunning checks if a container with the given name is running.
func (d *DockerClient) ContainerRunning(ctx context.Context, name string) (bool, error) {
	containers, err := d.cli.ContainerList(ctx, client.ContainerListOptions{
		All:     false, // Only running containers
		Filters: make(client.Filters).Add("name", name),
	})
	if err != nil {
		return false, err
	}
	return len(containers.Items) > 0, nil
}

// WaitForContainer waits for a container to be in the expected state.
func (d *DockerClient) WaitForContainer(ctx context.Context, name string, running bool) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			isRunning, err := d.ContainerRunning(ctx, name)
			if err != nil {
				return err
			}
			if isRunning == running {
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// CleanupCardinalContainers removes all Cardinal-related containers.
func CleanupCardinalContainers(ctx context.Context) error {
	dockerCli, err := NewDockerClient()
	if err != nil {
		return err
	}
	defer dockerCli.Close()

	containers, err := dockerCli.ListCardinalContainers(ctx)
	if err != nil {
		return err
	}

	for _, c := range containers {
		// Force remove the container
		if _, err := dockerCli.cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		}); err != nil {
			// Log but continue - container might already be removed
			continue
		}
	}

	return nil
}

// NetworkExists checks if a Docker network exists.
func (d *DockerClient) NetworkExists(ctx context.Context, name string) (bool, error) {
	networks, err := d.cli.NetworkList(ctx, client.NetworkListOptions{
		Filters: make(client.Filters).Add("name", name),
	})
	if err != nil {
		return false, err
	}
	return len(networks.Items) > 0, nil
}

// WaitForNATS waits until the project's NATS is reachable on port 4222.
// projectDir only names the container to dump on failure; pass "" to skip that.
func WaitForNATS(timeout time.Duration, projectDir string) bool {
	addr := "localhost:4222"
	fmt.Printf("DEBUG: WaitForNATS trying to connect to %s\n", addr)

	result := waitForPort(addr, timeout)

	if !result {
		fmt.Printf("DEBUG: WaitForNATS failed to connect to %s after %v\n", addr, timeout)
		if name, err := NatsContainerName(projectDir); err == nil {
			logNATSContainerState(name)
		}
	}
	return result
}

// WaitForNATSDown waits until NATS is no longer reachable on port 4222.
func WaitForNATSDown(timeout time.Duration) bool {
	return waitForPortDown("localhost:4222", timeout)
}

// waitForPort waits until a TCP port is reachable.
func waitForPort(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

// waitForPortDown waits until a TCP port is no longer reachable.
func waitForPortDown(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return true
		}
		conn.Close()
		time.Sleep(time.Second)
	}
	return false
}

// WaitForCardinalDebugReady polls the Cardinal debug ConnectRPC service until
// it responds successfully to an Introspect RPC. The host port is resolved by
// inspecting the Docker container's port bindings, falling back to the first
// instance's port if the container is not found.
func WaitForCardinalDebugReady(t testing.TB, timeout time.Duration, containerName string) bool {
	t.Helper()

	port := service.ShardHostPortBase
	if dockerClient, err := dockerpkg.NewClient(&service.Config{}, nil); err == nil {
		defer dockerClient.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if hp, err := dockerClient.CardinalHostPort(ctx, containerName); err == nil {
			port = hp
		}
		cancel()
	}

	debugURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	httpClient := &http.Client{Timeout: 2 * time.Second}
	debugClient := cardinalv1connect.NewDebugServiceClient(httpClient, debugURL)

	t.Logf("WaitForCardinalDebugReady polling %s (container %s)", debugURL, containerName)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := debugClient.Introspect(ctx, connect.NewRequest(&cardinalv1.IntrospectRequest{}))
		cancel()
		if err == nil {
			t.Logf("Cardinal debug service ready at %s", debugURL)
			return true
		}
		time.Sleep(2 * time.Second)
	}

	t.Logf("WaitForCardinalDebugReady timed out after %v for %s", timeout, debugURL)
	return false
}

// logNATSContainerState logs the project's NATS container state for debugging.
func logNATSContainerState(natsContainer string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Printf("DEBUG: failed to create docker client: %v\n", err)
		return
	}
	defer cli.Close()

	// Check if NATS container exists
	inspectResult, err := cli.ContainerInspect(ctx, natsContainer, client.ContainerInspectOptions{})
	if err != nil {
		fmt.Printf("DEBUG: NATS container inspect failed: %v\n", err)
		return
	}
	inspect := inspectResult.Container

	fmt.Printf("DEBUG: NATS container exists, ID: %s\n", inspect.ID[:12])
	if inspect.State != nil {
		fmt.Printf("DEBUG: NATS container state: %s, running: %v\n", inspect.State.Status, inspect.State.Running)
		if inspect.State.Health != nil {
			fmt.Printf("DEBUG: NATS health status: %s\n", inspect.State.Health.Status)
		}
	}

	// Get network settings
	if inspect.NetworkSettings != nil {
		for netName, netSettings := range inspect.NetworkSettings.Networks {
			fmt.Printf("DEBUG: NATS network %s: IP=%s\n", netName, netSettings.IPAddress)
		}
		fmt.Printf("DEBUG: NATS ports: %v\n", inspect.NetworkSettings.Ports)
	}

	// Get last 10 lines of logs
	logsOpts := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "10",
	}
	reader, err := cli.ContainerLogs(ctx, natsContainer, logsOpts)
	if err != nil {
		fmt.Printf("DEBUG: failed to get NATS logs: %v\n", err)
		return
	}
	defer reader.Close()

	logs, _ := io.ReadAll(reader)
	if len(logs) > 0 {
		fmt.Printf("DEBUG: NATS last logs:\n%s\n", string(logs))
	}
}
