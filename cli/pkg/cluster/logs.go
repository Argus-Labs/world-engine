package cluster

import (
	"bufio"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rotisserie/eris"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

const reconnectBackoff = 2 * time.Second

// StreamShardLogs tails the shard pods in opts.Namespace (filtered by ShardIDs /
// InstanceNames) into out until ctx ends, following each instance across pod rolls.
func (c *Client) StreamShardLogs(ctx context.Context, opts worldstatus.LogsOpts, out chan<- worldstatus.LogLine) error {
	defer close(out)
	if opts.Namespace == "" {
		return eris.New("StreamShardLogs: namespace is required")
	}
	cs, err := c.kube(ctx)
	if err != nil {
		return err
	}
	pods, err := cs.CoreV1().Pods(opts.Namespace).List(ctx, metav1.ListOptions{LabelSelector: shardIDLabel})
	if err != nil {
		return eris.Wrapf(err, "list shard pods in %s", opts.Namespace)
	}
	instances := map[string]string{}
	for i := range pods.Items {
		inst, shard := pods.Items[i].Labels[instanceLabel], pods.Items[i].Labels[shardIDLabel]
		if inst != "" && wantInstance(opts, shard, inst) {
			instances[inst] = shard
		}
	}
	if len(instances) == 0 {
		return eris.Errorf("no shard pods in namespace %s", opts.Namespace)
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(instances))
	for inst, shard := range instances {
		selector := shardIDLabel + "," + instanceLabel + "=" + inst
		lines := make(chan worldstatus.LogLine)
		wg.Go(func() {
			errs <- streamInstance(ctx, cs, opts, shard, inst, selector, lines)
			close(lines)
		})
		wg.Go(func() {
			for l := range lines {
				select {
				case <-ctx.Done():
				case out <- l:
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	var all []error
	for err := range errs {
		if err != nil && ctx.Err() == nil {
			all = append(all, err)
		}
	}
	return errors.Join(all...)
}

func wantInstance(opts worldstatus.LogsOpts, shardID, instance string) bool {
	if len(opts.ShardIDs) > 0 && !slices.Contains(opts.ShardIDs, shardID) {
		return false
	}
	return len(opts.InstanceNames) == 0 || slices.Contains(opts.InstanceNames, instance)
}

// PodLogs returns up to tail lines of one pod.
func (c *Client) PodLogs(ctx context.Context, namespace, podName string, tail int32) (string, error) {
	cs, err := c.kube(ctx)
	if err != nil {
		return "", err
	}
	opts := &corev1.PodLogOptions{}
	if tail > 0 {
		n := int64(tail)
		opts.TailLines = &n
	}
	data, err := cs.CoreV1().Pods(namespace).GetLogs(podName, opts).DoRaw(ctx)
	if err != nil {
		return "", eris.Wrapf(err, "logs for pod %s", podName)
	}
	return string(data), nil
}

// streamInstance follows the current pod of one instance, moving to its replacement when it rolls.
func streamInstance(
	ctx context.Context, cs kubernetes.Interface, opts worldstatus.LogsOpts,
	shard, inst, selector string, out chan<- worldstatus.LogLine,
) error {
	if opts.FollowWindow > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.FollowWindow)
		defer cancel()
	}
	current, err := pickPod(ctx, cs, opts.Namespace, selector)
	if err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := streamPod(ctx, cs, opts, shard, inst, current, out)
		if err == nil || ctx.Err() != nil {
			return nil
		}
		if !apierrors.IsNotFound(err) && !apierrors.IsBadRequest(err) {
			return err
		}
		next, perr := pickPod(ctx, cs, opts.Namespace, selector)
		if perr != nil || next == current {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(reconnectBackoff):
				continue
			}
		}
		current = next
		opts.TailLines, opts.Previous = 0, false // history was replayed on the first stream
	}
}

func pickPod(ctx context.Context, cs kubernetes.Interface, ns, selector string) (string, error) {
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", eris.Wrapf(err, "list pods %s in %s", selector, ns)
	}
	for i := range pods.Items {
		if pods.Items[i].Status.Phase == corev1.PodRunning {
			return pods.Items[i].Name, nil
		}
	}
	if len(pods.Items) > 0 {
		return pods.Items[0].Name, nil
	}
	return "", eris.Errorf("no pod for %s in %s", selector, ns)
}

func streamPod(
	ctx context.Context, cs kubernetes.Interface, opts worldstatus.LogsOpts,
	shard, inst, podName string, out chan<- worldstatus.LogLine,
) error {
	logOpts := &corev1.PodLogOptions{Follow: !opts.Previous, Timestamps: true, Previous: opts.Previous}
	if opts.TailLines > 0 {
		n := int64(opts.TailLines)
		logOpts.TailLines = &n
	}
	rc, err := cs.CoreV1().Pods(opts.Namespace).GetLogs(podName, logOpts).Stream(ctx)
	if err != nil {
		return eris.Wrapf(err, "open log stream for %s/%s", opts.Namespace, podName)
	}
	defer func() { _ = rc.Close() }()

	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // long zerolog JSON lines
	for scanner.Scan() {
		ts, line := splitTimestamp(scanner.Text())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- worldstatus.LogLine{ShardID: shard, InstanceName: inst, Source: podName, Timestamp: ts, Line: line}:
		}
	}
	if err := scanner.Err(); err != nil && !eris.Is(err, io.EOF) {
		return err
	}
	return nil
}

// splitTimestamp peels the RFC3339Nano prefix kubelet adds with Timestamps=true.
func splitTimestamp(line string) (string, string) {
	idx := strings.IndexByte(line, ' ')
	if idx <= 0 {
		return "", line
	}
	return line[:idx], line[idx+1:]
}
