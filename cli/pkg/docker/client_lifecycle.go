package docker

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/rotisserie/eris"
	"golang.org/x/sync/errgroup"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

const (
	readyTimeout = 5 * time.Minute
	probeTimeout = 2 * time.Second
	// crashLoopRestarts is how many policy restarts during a readiness wait mean a
	// crash loop. Earlier restarts may be a dependency that was not up yet.
	crashLoopRestarts = 3
	// stablePolls is how many consecutive running polls a probe-less container needs
	// before it counts as ready, so a process that dies after a second is caught.
	stablePolls = 3

	// ContainerRunning is Docker's running container state.
	ContainerRunning = "running"
)

// probeClient has no keep-alives: each probe must make a fresh connection, or a pooled
// one to a container that has since died would answer for it.
//
//nolint:gochecknoglobals // a stateless client shared by every readiness probe
var probeClient = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

// StartContainers creates (if missing) and starts containers in dependency order:
// NATS, then the project DB, then [[services]] in declaration order (each waiting
// for readiness), then shards concurrently. progress may be nil.
func (c *Client) StartContainers(ctx context.Context, services []service.Service, progress func(Progress)) error {
	progress = synchronized(progress)
	byRole := map[string][]service.Service{}
	for _, s := range services {
		byRole[s.Labels[service.RoleLabel]] = append(byRole[s.Labels[service.RoleLabel]], s)
	}

	startAndWait := func(svc service.Service) error {
		notify(progress, Progress{Name: svc.Name, State: StateStarting})
		if err := c.startContainer(ctx, svc); err != nil {
			notify(progress, Progress{Name: svc.Name, State: StateStarting, Err: err})
			return err
		}
		if err := c.waitForReady(ctx, svc); err != nil {
			notify(progress, Progress{Name: svc.Name, State: StateStarting, Err: err})
			return err
		}
		notify(progress, Progress{Name: svc.Name, State: StateStarted})
		return nil
	}

	for _, role := range []string{service.RoleNATS, service.RoleDB, service.RoleService} {
		for _, svc := range byRole[role] {
			if err := startAndWait(svc); err != nil {
				return eris.Wrapf(err, "start %s", svc.Name)
			}
		}
	}

	var g errgroup.Group
	for _, svc := range byRole[service.RoleShard] {
		g.Go(func() error {
			notify(progress, Progress{Name: svc.Name, State: StateStarting})
			if err := c.startContainer(ctx, svc); err != nil {
				notify(progress, Progress{Name: svc.Name, State: StateStarting, Err: err})
				return eris.Wrapf(err, "start %s", svc.Name)
			}
			notify(progress, Progress{Name: svc.Name, State: StateStarted})
			return nil
		})
	}
	return g.Wait()
}

// waitForReady blocks until the container's Docker health is "healthy", or its
// ReadyURL answers, or it is running and declares neither. It delegates to
// waitContainerReady, polling the Docker state once per second.
func (c *Client) waitForReady(ctx context.Context, svc service.Service) error {
	return waitContainerReady(ctx, c.client, c.logContainerDebugInfo, time.Second, svc)
}

// waitContainerReady polls cli every poll until the container is ready, ctx ends,
// or it crashed. Platform containers run unless-stopped, so a boot crash shows as
// "restarting" with a growing RestartCount rather than "exited"; crashLoopRestarts
// restarts since this start fail the wait, with the exit code when a poll saw it.
// Shards (policy "no") are started without this wait. logDebug dumps state and
// tail logs on failure; it is a callback so the loop can run against a fake daemon.
func waitContainerReady(
	ctx context.Context,
	cli dockerContainerClient,
	logDebug func(name string),
	poll time.Duration,
	svc service.Service,
) error {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	// RestartCount is lifetime and survives a start of an existing container, so count from here.
	res, err := cli.ContainerInspect(ctx, svc.Name, client.ContainerInspectOptions{})
	if err != nil {
		return eris.Wrapf(err, "inspect %s", svc.Name)
	}
	base := res.Container.RestartCount
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	stable, lastRestarts := 0, 0
	lastExit, sawExit := 0, false // Docker resets ExitCode to 0 on each policy restart
	for {
		select {
		case <-ctx.Done():
			logDebug(svc.Name)
			return eris.Wrapf(ctx.Err(), "timeout waiting for %s to become ready", svc.Name)
		case <-ticker.C:
			res, err := cli.ContainerInspect(ctx, svc.Name, client.ContainerInspectOptions{})
			if err != nil {
				return eris.Wrapf(err, "inspect %s", svc.Name)
			}
			st := res.Container.State
			if st == nil {
				continue
			}
			restarts := res.Container.RestartCount - base
			if st.Restarting {
				lastExit, sawExit = st.ExitCode, true
			}
			if restarts >= crashLoopRestarts {
				logDebug(svc.Name)
				if sawExit {
					return eris.Errorf("%s crash-looping: %d restarts, last exit code %d", svc.Name, restarts, lastExit)
				}
				return eris.Errorf("%s crash-looping: %d restarts", svc.Name, restarts)
			}
			if st.Status == container.StateExited {
				logDebug(svc.Name)
				return eris.Errorf("%s exited with code %d", svc.Name, st.ExitCode)
			}
			if restarts != lastRestarts || !st.Running || st.Restarting {
				lastRestarts, stable = restarts, 0
				continue
			}
			switch {
			case st.Health != nil:
				if st.Health.Status == container.Healthy {
					return nil
				}
			case svc.ReadyURL != "":
				if httpReady(ctx, svc.ReadyURL) {
					return nil
				}
			default:
				if stable++; stable >= stablePolls {
					return nil
				}
			}
		}
	}
}

// httpReady reports whether url answers 2xx. Docker's port proxy accepts a TCP
// connection the moment the container starts, so only a reply from the process inside
// proves it is serving, and NATS's /healthz answers non-2xx while it is still unhealthy.
func httpReady(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

// logContainerDebugInfo logs state and the last log lines after a readiness failure.
func (c *Client) logContainerDebugInfo(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if st, err := c.ContainerState(ctx, name); err == nil {
		c.logger.Warn(
			"container state",
			"container",
			name,
			"status",
			st.Status,
			"health",
			st.Health,
			"exit",
			st.ExitCode,
		)
	}
	if logs, err := c.GetContainerLogs(ctx, name, 10, 0); err == nil && logs != "" {
		c.logger.Warn("container logs", "container", name, "logs", logs)
	}
}

// StopContainers stops containers concurrently; missing ones are skipped. Volumes stay.
func (c *Client) StopContainers(ctx context.Context, names []string, progress func(Progress)) error {
	progress = synchronized(progress)
	var g errgroup.Group
	for _, name := range names {
		g.Go(func() error {
			notify(progress, Progress{Name: name, State: StateStopping})
			if err := c.stopContainer(ctx, name); err != nil {
				notify(progress, Progress{Name: name, State: StateStopping, Err: err})
				return err
			}
			notify(progress, Progress{Name: name, State: StateStopped})
			return nil
		})
	}
	return g.Wait()
}

// RemoveContainers stops and removes containers concurrently; keepVolumes false
// also removes their named volumes.
func (c *Client) RemoveContainers(
	ctx context.Context,
	names []string,
	keepVolumes bool,
	progress func(Progress),
) error {
	progress = synchronized(progress)
	var g errgroup.Group
	for _, name := range names {
		g.Go(func() error {
			notify(progress, Progress{Name: name, State: StateRemoving})
			if err := c.removeContainer(ctx, name, keepVolumes); err != nil {
				notify(progress, Progress{Name: name, State: StateRemoving, Err: err})
				return err
			}
			notify(progress, Progress{Name: name, State: StateRemoved})
			return nil
		})
	}
	return g.Wait()
}

// ContainerState is the subset of docker inspect that status and readiness read.
type ContainerState struct {
	Name         string
	Image        string
	Running      bool
	Status       string // created, running, exited, ...
	Health       string // "" when the image has no healthcheck
	ExitCode     int
	StartedAt    time.Time
	RestartCount int
	Labels       map[string]string
}

// ContainerState inspects one container by name.
func (c *Client) ContainerState(ctx context.Context, name string) (ContainerState, error) {
	res, err := c.client.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		return ContainerState{}, eris.Wrapf(err, "inspect %s", name)
	}
	cont := res.Container
	out := ContainerState{Name: name, RestartCount: cont.RestartCount}
	if cont.Config != nil {
		out.Image = cont.Config.Image
		out.Labels = cont.Config.Labels
	}
	if st := cont.State; st != nil {
		out.Running = st.Running
		out.Status = string(st.Status)
		out.ExitCode = st.ExitCode
		if st.Health != nil {
			out.Health = string(st.Health.Status)
		}
		if t, err := time.Parse(time.RFC3339Nano, st.StartedAt); err == nil {
			out.StartedAt = t
		}
	}
	return out, nil
}

// ContainerSummary is one row of docker ps for a project.
type ContainerSummary struct {
	Name   string
	Image  string
	State  string // running, exited, ...
	Labels map[string]string
}

// ListProjectContainers returns every container (running or not) labelled with the project.
func (c *Client) ListProjectContainers(ctx context.Context, project string) ([]ContainerSummary, error) {
	res, err := c.client.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", service.ProjectLabel+"="+project),
	})
	if err != nil {
		return nil, eris.Wrapf(err, "list containers for project %q", project)
	}
	out := make([]ContainerSummary, 0, len(res.Items))
	for _, it := range res.Items {
		name := ""
		if len(it.Names) > 0 {
			name = strings.TrimPrefix(it.Names[0], "/")
		}
		out = append(out, ContainerSummary{Name: name, Image: it.Image, State: string(it.State), Labels: it.Labels})
	}
	return out, nil
}
