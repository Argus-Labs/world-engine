package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rotisserie/eris"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const (
	// natsClientPort is the NATS client port (matches manifests/nats/nats.yaml).
	// NATS is ClusterIP-only — not published on the host like the gateway/operator
	// (see k3d.go) — so host-side ops reach it over a short-lived port-forward.
	natsClientPort = 4222

	// shardPodsGoneTimeout bounds how long PurgeWorldState waits for pods to
	// terminate before wiping snapshots, so a shutting-down shard can't write a
	// final snapshot that resurrects a just-deleted bucket.
	shardPodsGoneTimeout = 45 * time.Second

	// snapshotBucketSuffix is the trailing segment of world-engine's JetStream
	// snapshot Object Store buckets, named "{org}_{project}_{shardID}_snapshot"
	// (see world-engine pkg/cardinal/snapshot/storage_jetstream.go).
	snapshotBucketSuffix = "_snapshot"

	// shardPodSelector matches only shard pods: the operator stamps
	// managed-by=cardinal-operator on the instances it owns, but not on itself, the
	// project DB, or [[services]] sharing the namespace. Mirrors the operator's
	// labels.{ManagedBy,OperatorName} (internal to that module); keep in lockstep.
	shardPodSelector = "app.kubernetes.io/managed-by=cardinal-operator"

	// shardIDLabel is a pod's pool id (e.g. "gameplay"), shared by every replica —
	// use it to aggregate per pool, never to select one instance.
	// Mirrors the operator's labels.ShardID; keep in lockstep.
	shardIDLabel = "cardinal.argus.gg/shard-id"

	// instanceLabel is a pod's per-replica name (e.g. "gameplay-2"). Unlike
	// shardIDLabel it identifies one instance, so draining on shardIDLabel instead
	// matches zero pods. Mirrors the operator's labels.Instance; keep in lockstep.
	instanceLabel = "cardinal.argus.gg/instance"
)

// instancePodSelector scopes shardPodSelector to a single instance's pods.
func instancePodSelector(instanceID string) string {
	return fmt.Sprintf("%s,%s=%s", shardPodSelector, instanceLabel, instanceID)
}

// PurgeWorldState wipes one world's persisted state so its next deploy starts on
// a fresh tick: it deletes every JetStream snapshot bucket under the
// "{org}_{project}_" prefix (also catching shards since removed from config, which
// a per-shard match would miss). Assumes no two local worlds share that prefix —
// safe one-world-at-a-time.
//
// Call AFTER undeploying the shards: it first waits for this world's shard pods to
// terminate so a shutting-down shard can't re-snapshot over the wipe.
func (c *Client) PurgeWorldState(ctx context.Context, org, project string) error {
	if org == "" || project == "" {
		return eris.New("PurgeWorldState: org and project are required")
	}

	k, cs, err := c.kubeAndClientset(ctx)
	if err != nil {
		return err
	}

	// No shard may be alive to re-create a bucket after we delete it. A drain timeout
	// is fatal: wiping under a live shard lets it re-snapshot over the wipe.
	if err := waitForPodsGone(ctx, cs, operatorNamespace, shardPodSelector, shardPodsGoneTimeout); err != nil {
		return err
	}

	js, cleanup, err := connectJetStream(ctx, k, cs)
	if err != nil {
		return err
	}
	defer cleanup()

	// Drain the lister fully before deleting (delete is request/reply over the
	// same connection the lister streams on).
	prefix := fmt.Sprintf("%s_%s_", org, project)
	lister := js.ObjectStoreNames(ctx)
	var targets []string
	for name := range lister.Name() {
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, snapshotBucketSuffix) {
			targets = append(targets, name)
		}
	}
	if err := lister.Error(); err != nil {
		return eris.Wrap(err, "list snapshot buckets")
	}

	for _, name := range targets {
		// ErrBucketNotFound: a concurrent delete already removed it — fine.
		// Must use stdlib errors.Is: eris.Is (v0.5.4) can't see through
		// DeleteObjectStore's errors.Join wrapping (it only follows
		// single-error Unwrap), so it always returned false and this benign
		// case looked like a real failure.
		if err := js.DeleteObjectStore(ctx, name); err != nil && !errors.Is(err, jetstream.ErrBucketNotFound) {
			return eris.Wrapf(err, "delete snapshot bucket %s", name)
		}
	}
	return nil
}

// PurgeShardState is the single-instance analog of PurgeWorldState: it deletes only
// instanceID's JetStream snapshot bucket, leaving every other instance's state (and
// pods) untouched. Call AFTER UndeployShard: it first waits for that instance's pods
// to terminate, so a shutting-down replica can't re-snapshot over the wipe.
func (c *Client) PurgeShardState(ctx context.Context, org, project, instanceID string) error {
	if org == "" || project == "" || instanceID == "" {
		return eris.New("PurgeShardState: org, project, and instanceID are required")
	}

	k, cs, err := c.kubeAndClientset(ctx)
	if err != nil {
		return err
	}

	selector := instancePodSelector(instanceID)
	if err := waitForPodsGone(ctx, cs, operatorNamespace, selector, shardPodsGoneTimeout); err != nil {
		return err
	}

	js, cleanup, err := connectJetStream(ctx, k, cs)
	if err != nil {
		return err
	}
	defer cleanup()

	// A shard that never snapshotted has no bucket yet, so not-found is the normal
	// case here, not an edge one. See the errors.Is note in PurgeWorldState.
	bucket := fmt.Sprintf("%s_%s_%s%s", org, project, instanceID, snapshotBucketSuffix)
	if err := js.DeleteObjectStore(ctx, bucket); err != nil && !errors.Is(err, jetstream.ErrBucketNotFound) {
		return eris.Wrapf(err, "delete snapshot bucket %s", bucket)
	}
	return nil
}

// kubeAndClientset builds the kubeClient (for port-forwarding) alongside a
// plain client-go Clientset (for pod listing) — every purge path needs both.
func (c *Client) kubeAndClientset(ctx context.Context) (*kubeClient, kubernetes.Interface, error) {
	k, err := c.kube(ctx)
	if err != nil {
		return nil, nil, err
	}
	cs, err := kubernetes.NewForConfig(k.restCfg)
	if err != nil {
		return nil, nil, eris.Wrap(err, "build kubernetes clientset")
	}
	return k, cs, nil
}

// connectJetStream port-forwards to the in-cluster NATS server and returns a
// JetStream context, plus a cleanup that closes the connection and the
// port-forward. Callers defer cleanup() immediately.
func connectJetStream(
	ctx context.Context,
	k *kubeClient,
	cs kubernetes.Interface,
) (jetstream.JetStream, func(), error) {
	natsRef, err := natsPlatformRef()
	if err != nil {
		return nil, nil, err
	}
	podName, err := pickPlatformPod(ctx, cs, natsRef)
	if err != nil {
		return nil, nil, err
	}
	stopCh, localPort, err := k.portForward(ctx, cs, natsRef.Namespace, podName, natsClientPort)
	if err != nil {
		return nil, nil, err
	}

	nc, err := nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d", localPort))
	if err != nil {
		close(stopCh)
		return nil, nil, eris.Wrap(err, "connect to NATS")
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		close(stopCh)
		return nil, nil, eris.Wrap(err, "create jetstream context")
	}

	return js, func() { nc.Close(); close(stopCh) }, nil
}

// natsPlatformRef locates the in-cluster NATS server, reusing the single
// definition in PlatformPods so its namespace/selector can't drift.
func natsPlatformRef() (PlatformPodRef, error) {
	for _, ref := range PlatformPods() {
		if ref.Name == "nats" {
			return ref, nil
		}
	}
	return PlatformPodRef{}, eris.New("nats platform pod ref not registered")
}

// waitForPodsGone blocks until no pods matching selector remain in ns, or errors
// if they haven't terminated within timeout.
//
// The timeout errors rather than being best-effort: a caller about to wipe must not
// proceed while a matching pod is alive, or it can re-snapshot over the wipe and
// resurrect the tick the purge should reset.
func waitForPodsGone(ctx context.Context, cs kubernetes.Interface, ns, selector string, timeout time.Duration) error {
	var remaining int
	var lastListErr error
	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, timeout, true,
		func(ctx context.Context) (bool, error) {
			pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
			if err != nil {
				// Ignore our own deadline cancelling the List; keep real API errors.
				if ctx.Err() == nil {
					lastListErr = err
				}
				return false, nil
			}
			remaining = len(pods.Items)
			return remaining == 0, nil
		})
	if err == nil {
		return nil
	}
	if lastListErr != nil {
		return eris.Wrapf(lastListErr, "list pods (%s) in %s while draining", selector, ns)
	}
	return eris.Errorf("%d pod(s) matching %q still terminating in %s after %s", remaining, selector, ns, timeout)
}

// portForward forwards an ephemeral local port to remotePort on the named pod,
// returning a stop channel (close to tear down) and the chosen local port. The
// apiserver request path is built by client-go, not hand-assembled, so it tracks
// the client's API version.
func (k *kubeClient) portForward(
	ctx context.Context, cs kubernetes.Interface, ns, podName string, remotePort int,
) (chan struct{}, int, error) {
	roundTripper, upgrader, err := spdy.RoundTripperFor(k.restCfg)
	if err != nil {
		return nil, 0, eris.Wrap(err, "build spdy round tripper")
	}
	reqURL := cs.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(ns).Name(podName).SubResource("portforward").URL()

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, reqURL)

	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})
	pf, err := portforward.New(
		dialer, []string{fmt.Sprintf("0:%d", remotePort)}, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		close(stopCh)
		return nil, 0, eris.Wrap(err, "create port-forward")
	}

	errCh := make(chan error, 1)
	go func() { errCh <- pf.ForwardPorts() }()

	select {
	case <-readyCh:
	case err := <-errCh:
		close(stopCh)
		return nil, 0, eris.Wrap(err, "port-forward failed before ready")
	case <-ctx.Done():
		close(stopCh)
		return nil, 0, eris.Wrap(ctx.Err(), "port-forward cancelled")
	}

	ports, err := pf.GetPorts()
	if err != nil || len(ports) == 0 {
		close(stopCh)
		return nil, 0, eris.Wrap(err, "resolve forwarded local port")
	}
	return stopCh, int(ports[0].Local), nil
}
