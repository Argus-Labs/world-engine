package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/rotisserie/eris"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// podWaitTimeout bounds how long waitForServiceReady polls for a Ready pod
// before giving up. Cold-boot pods take ~10-30s; 60s leaves headroom.
const podWaitTimeout = 2 * time.Minute

// shardReadyTimeout bounds how long WaitForShardsReady polls for the freshly
// redeployed shard pods to become Ready. Generous because a purged shard also
// re-imports its image and boots from an empty snapshot before it reports Ready.
const shardReadyTimeout = 120 * time.Second

// Platform services whose Ready backing pods gate Start/DeployWorld progress.
const (
	natsNamespace    = "nats"
	natsService      = "nats"
	traefikNamespace = "traefik"
	traefikService   = "traefik"
	// operatorService is the operator Service name; operatorNamespace is defined
	// in bootstrap.go.
	operatorService = "cardinal-operator"
)

// waitForServiceReady blocks until a pod backing the named Service is Ready, so
// callers know the Service can actually route traffic.
func (k *kubeClient) waitForServiceReady(ctx context.Context, namespace, service string) error {
	cs, err := kubernetes.NewForConfig(k.restCfg)
	if err != nil {
		return eris.Wrap(err, "kubernetes clientset")
	}
	return waitForReadyPod(ctx, cs, namespace, service)
}

// waitForReadyPod polls until a Ready pod backs the named service. Service
// objects route to their pods, so a Ready backing pod means the Service (and
// its NodePort, if any) will actually serve traffic. Polls up to podWaitTimeout
// because Start can return before platform pods have rolled.
func waitForReadyPod(ctx context.Context, cs kubernetes.Interface, namespace, service string) error {
	svc, err := cs.CoreV1().Services(namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return eris.Wrapf(err, "get service %s/%s", namespace, service)
	}
	if len(svc.Spec.Selector) == 0 {
		return eris.New(fmt.Sprintf("service %s/%s has no selector", namespace, service))
	}
	selector := labels.SelectorFromSet(svc.Spec.Selector).String()

	var lastListErr error
	err = wait.PollUntilContextTimeout(ctx, time.Second, podWaitTimeout, true,
		func(ctx context.Context) (bool, error) {
			pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
			if err != nil {
				// Keep real API errors (RBAC, unreachable) for the timeout message;
				// ignore errors from our own deadline cancelling the call.
				if ctx.Err() == nil {
					lastListErr = err
				}
				return false, nil
			}
			for i := range pods.Items {
				if podReady(&pods.Items[i]) {
					return true, nil
				}
			}
			return false, nil
		})
	if err == nil {
		return nil
	}
	if lastListErr != nil {
		return eris.Wrapf(
			lastListErr,
			"no Ready pod backing service %s/%s: pod list kept failing within %s",
			namespace,
			service,
			podWaitTimeout,
		)
	}
	return eris.Wrapf(
		err,
		"no Ready pod backing service %s/%s after %s",
		namespace,
		service,
		podWaitTimeout,
	)
}

// podReady reports whether the kubelet has marked the pod Ready — its readiness
// probes pass and it's in Service rotation. This is strictly stronger than the
// Running phase, which only confirms a container process started.
func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// WaitForShardsReady blocks until every shard instance has a Ready pod running
// the just-deployed image:tag, or shardReadyTimeout elapses, so a caller can act
// on the shards without racing a still-ContainerCreating pod. Matching the new tag
// keeps a rolling redeploy honest — the old-tag pod stays Ready for a beat but
// doesn't count.
//
// onProgress, if non-nil, gets the current ready/expected pod count after every
// poll tick — lets a caller show live "N/M ready" progress instead of one static
// spinner for the whole wait.
//
// Best-effort (no error): the deploy already succeeded, so a readiness hiccup
// must never fail it; a wedged shard still surfaces after the timeout.
func (c *Client) WaitForShardsReady(ctx context.Context, cfg toml.Config, onProgress func(ready, expected int)) {
	pools, err := c.ShardPoolsFromConfig(cfg)
	if err != nil {
		return
	}
	expected := 0
	for _, p := range pools {
		expected += int(p.PoolSize)
	}
	if expected <= 0 {
		return
	}

	k, cs, err := c.kubeAndClientset(ctx)
	if err != nil {
		return
	}

	// The Deploy RPC has already stamped spec.imageTag on every ShardPool, so the
	// desired refs are fixed for this wait — read them once. An unreadable/empty
	// set means there's nothing to gate on, so don't block.
	desired, err := k.shardPoolImages(ctx, operatorNamespace)
	if err != nil || len(desired) == 0 {
		return
	}

	// Best-effort: discard the poll's terminal error (timeout / ctx cancel) — a
	// shard that isn't Ready by the deadline is left for the caller to observe.
	_ = wait.PollUntilContextTimeout(ctx, time.Second, shardReadyTimeout, true,
		func(ctx context.Context) (bool, error) {
			ready := countReadyPoolPods(ctx, cs, pools, desired)
			if onProgress != nil {
				onProgress(ready, expected)
			}
			return ready >= expected, nil
		})
}

// countReadyPoolPods returns how many of pools' pods are Ready on a desired
// image:tag, capped per pool so one pool's surplus can't mask another's. Bucketing
// by shardIDLabel keeps a ShardPool dropped from world.toml out of a declared
// pool's count. A list error counts as zero, so the caller retries.
func countReadyPoolPods(
	ctx context.Context,
	cs kubernetes.Interface,
	pools []ShardPool,
	desired map[string]struct{},
) int {
	pods, err := cs.CoreV1().Pods(operatorNamespace).List(
		ctx, metav1.ListOptions{LabelSelector: shardPodSelector})
	if err != nil {
		return 0
	}
	byShard := make(map[string]int, len(pools))
	for i := range pods.Items {
		pod := &pods.Items[i]
		if podRunsDesiredImage(pod, desired) && podReady(pod) {
			byShard[pod.Labels[shardIDLabel]]++
		}
	}
	ready := 0
	for _, p := range pools {
		ready += min(byShard[p.ShardID], int(p.PoolSize))
	}
	return ready
}

// podRunsDesiredImage reports whether any of the pod's containers runs one of the
// desired "image:tag" refs — i.e. the pod belongs to the current deployment, not
// a still-terminating old one from a rolling reload.
func podRunsDesiredImage(p *corev1.Pod, desired map[string]struct{}) bool {
	for i := range p.Spec.Containers {
		if _, ok := desired[p.Spec.Containers[i].Image]; ok {
			return true
		}
	}
	return false
}
