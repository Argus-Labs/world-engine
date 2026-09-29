package cluster

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"

	"github.com/rotisserie/eris"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// PlatformPodRef identifies a platform component for log streaming. Unlike
// shards — which the cardinal-operator enumerates via its Status RPC — these run
// in their own namespaces and aren't owned by any ShardPool, so the world-cli
// has to talk to the kube-apiserver directly.
type PlatformPodRef struct {
	Name      string // display label, used as the LogLine.InstanceName tag
	Namespace string
	Selector  string // label selector (e.g. "app=nats")
}

// PlatformPods returns the platform components the picker offers, in display order.
func PlatformPods() []PlatformPodRef {
	return []PlatformPodRef{
		{
			Name:      "traefik",
			Namespace: traefikNamespace,
			Selector:  "app.kubernetes.io/name=traefik,app.kubernetes.io/instance=traefik-traefik",
		},
		{Name: "nats", Namespace: natsNamespace, Selector: "app=nats"},
	}
}

// ProjectServicePods returns the per-project [[services]] + auto project DB the
// picker offers, in declaration order with the DB last. Like Traefik/NATS,
// these aren't ShardPool-owned, so they stream via StreamPlatformLogs (direct
// kube-apiserver) — no port-forward involved. The auto "{project}-db" is listed
// whenever DeployWorld provisions it — i.e. when no config_db service brings its
// own (the same gate as client.go's DeployWorld), so any deployed auto DB is
// streamable. With a config_db service, only the declared [[services]] appear.
func ProjectServicePods(project string, cfg toml.Config) []PlatformPodRef {
	refs := make([]PlatformPodRef, 0, len(cfg.Services)+1)
	for _, svc := range cfg.Services {
		refs = append(refs, PlatformPodRef{
			Name:      svc.ID,
			Namespace: projectServiceNamespace(),
			Selector:  "app=" + serviceContainerName(project, svc.ID),
		})
	}
	if !hasConfigDBService(cfg) {
		refs = append(refs, PlatformPodRef{
			Name:      "db",
			Namespace: projectServiceNamespace(),
			Selector:  "app=" + projectDBName(project),
		})
	}
	return refs
}

// StreamPlatformLogs tails the current pod for the given platform component
// by hitting the kube-apiserver's Pods.GetLogs endpoint directly. Closes out
// on return.
//
// On pod-gone (rolling restart, eviction), re-resolves the selector and
// re-subscribes. ctx cancellation stops the stream. opts.FollowWindow bounds
// only the follow phase, not the initial pod discovery.
//
// This deliberately bypasses cardinal-operator: the operator's StreamPodLogs is
// hard-coded to a single namespace (the one it watches for ShardPools), so it
// can't reach platform pods outside that namespace.
func (c *Client) StreamPlatformLogs(ctx context.Context, ref PlatformPodRef, opts LogsOpts, out chan<- LogLine) error {
	defer close(out)

	k, err := c.kube(ctx)
	if err != nil {
		return err
	}
	cs, err := kubernetes.NewForConfig(k.restCfg)
	if err != nil {
		return eris.Wrap(err, "build kubernetes clientset")
	}

	current, err := pickPlatformPod(ctx, cs, ref)
	if err != nil {
		return err
	}

	if opts.FollowWindow > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.FollowWindow)
		defer cancel()
	}

	for {
		if ctx.Err() != nil {
			return nil
		}
		err := streamPlatformPod(ctx, cs, ref, current, opts, out)
		if err == nil || ctx.Err() != nil {
			return nil
		}
		// Pod may have rolled; try to find a replacement.
		if !isPlatformPodGone(err) {
			return err
		}
		next, perr := pickPlatformPod(ctx, cs, ref)
		if perr != nil || next == current {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(reconnectBackoff):
				continue
			}
		}
		current = next
		opts.TailLines = 0 // history was already replayed on the original stream
		opts.Previous = false
	}
}

// pickPlatformPod returns the first Running pod backing the platform
// component. Polls briefly because Start can return before the platform pods
// have rolled.
func pickPlatformPod(ctx context.Context, cs kubernetes.Interface, ref PlatformPodRef) (string, error) {
	var podName string
	var lastListErr error
	err := wait.PollUntilContextTimeout(ctx, time.Second, podWaitTimeout, true,
		func(ctx context.Context) (bool, error) {
			pods, err := cs.CoreV1().Pods(ref.Namespace).List(ctx, metav1.ListOptions{LabelSelector: ref.Selector})
			if err != nil {
				// Ignore our own deadline cancelling the List; keep real API
				// failures (RBAC, unreachable) for the timeout message.
				if ctx.Err() == nil {
					lastListErr = err
				}
				return false, nil
			}
			for i := range pods.Items {
				if pods.Items[i].Status.Phase == corev1.PodRunning {
					podName = pods.Items[i].Name
					return true, nil
				}
			}
			return false, nil
		})
	if err == nil {
		return podName, nil
	}
	// A cancelled/expired parent context (e.g. a bounded log window ending) is a
	// normal stop, not a pod-availability failure — surface it as the ctx error so
	// callers can tell it from "the pod never ran" via context.Canceled/DeadlineExceeded.
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if lastListErr != nil {
		return "", eris.Wrapf(lastListErr, "list pods %s/%s", ref.Namespace, ref.Selector)
	}
	return "", eris.Errorf("no running pod for %s/%s within %s", ref.Namespace, ref.Selector, podWaitTimeout)
}

// streamPlatformPod opens one GetLogs subscription and pumps lines into out.
// Returns nil when the server closes the stream cleanly.
func streamPlatformPod(
	ctx context.Context,
	cs kubernetes.Interface,
	ref PlatformPodRef,
	podName string,
	opts LogsOpts,
	out chan<- LogLine,
) error {
	tailLines := int64(opts.TailLines)
	logOpts := &corev1.PodLogOptions{
		Follow:     !opts.Previous,
		Timestamps: true,
		Previous:   opts.Previous,
	}
	if tailLines > 0 {
		logOpts.TailLines = &tailLines
	}

	req := cs.CoreV1().Pods(ref.Namespace).GetLogs(podName, logOpts)
	rc, err := req.Stream(ctx)
	if err != nil {
		return eris.Wrapf(err, "open log stream for %s/%s", ref.Namespace, podName)
	}
	defer func() { _ = rc.Close() }()

	scanner := bufio.NewScanner(rc)
	// Default 64KB buffer can choke on long zerolog JSON lines (events with
	// large embedded payloads). Match the operator's buffer.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		ts, line := splitTimestamp(scanner.Text())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- LogLine{
			ShardID:      ref.Name,
			InstanceName: ref.Name,
			PodName:      podName,
			Timestamp:    ts,
			Line:         line,
		}:
		}
	}
	if err := scanner.Err(); err != nil && !eris.Is(err, io.EOF) {
		return err
	}
	return nil
}

// splitTimestamp peels the leading RFC3339Nano timestamp that kubelet
// prepends when Timestamps=true. Format is "<ts> <line>". If the line
// doesn't match (rare; usually a kubelet quirk), returns ts="" + the full
// line.
func splitTimestamp(line string) (ts, rest string) {
	idx := strings.IndexByte(line, ' ')
	if idx <= 0 {
		return "", line
	}
	return line[:idx], line[idx+1:]
}

// isPlatformPodGone matches the various ways the kube-apiserver signals a
// pod has disappeared (NotFound, BadRequest-after-eviction).
func isPlatformPodGone(err error) bool {
	return apierrors.IsNotFound(err) || apierrors.IsBadRequest(err)
}
