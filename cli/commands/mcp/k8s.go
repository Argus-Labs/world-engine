package mcp

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	operatorv1 "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1"
	"github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1/operatorv1connect"
)

// platformLogWindow bounds how long a NATS log fetch follows the live
// pod. StreamPlatformLogs replays the historical tail immediately, then follows;
// we let it run for this window to drain the tail, then the context expires and
// the stream returns cleanly.
const platformLogWindow = 3 * time.Second

// defaultOperatorURL is the local cardinal-operator endpoint (k3d NodePort).
func defaultOperatorURL() string { return cluster.Defaults().OperatorEndpoint }

// newOperatorClient returns a Connect client for the local cardinal-operator.
// The local operator is unauthenticated; the client has no timeout so
// server-streaming RPCs (StreamPodLogs) aren't cut off mid-stream — callers
// bound those via context.
func newOperatorClient(operatorURL string) operatorv1connect.OperatorServiceClient {
	if operatorURL == "" {
		operatorURL = defaultOperatorURL()
	}
	return operatorv1connect.NewOperatorServiceClient(&http.Client{}, operatorURL)
}

// operatorStatus fetches the operator's pool/instance status, the source of
// truth for shard state in the cluster.
func operatorStatus(ctx context.Context, operatorURL string) (*operatorv1.StatusResponse, error) {
	resp, err := newOperatorClient(operatorURL).Status(ctx, connect.NewRequest(&operatorv1.StatusRequest{}))
	if err != nil {
		return nil, eris.Wrap(err, "operator status request failed")
	}
	return resp.Msg, nil
}

// toShardPools converts the operator's status response into the MCP status shape.
func toShardPools(status *operatorv1.StatusResponse) []ShardPool {
	pools := make([]ShardPool, 0, len(status.GetPools()))
	for _, p := range status.GetPools() {
		instances := make([]ShardInstance, 0, len(p.GetInstances()))
		for _, inst := range p.GetInstances() {
			instances = append(instances, ShardInstance{
				Name:         inst.GetName(),
				PodName:      inst.GetPodName(),
				Phase:        inst.GetPhase(),
				Ready:        inst.GetReady(),
				RestartCount: inst.GetRestartCount(),
				Age:          inst.GetAge(),
			})
		}
		pools = append(pools, ShardPool{
			ShardID:   p.GetShardId(),
			Namespace: p.GetNamespace(),
			PoolSize:  p.GetPoolSize(),
			ImageTag:  p.GetImageTag(),
			Phase:     p.GetPhase(),
			Instances: instances,
		})
	}
	return pools
}

// poolFor returns a shard's operator pool, or nil when the shard has none. Scoping instance
// lookups to the shard's own pool is what keeps a reference like "2" from resolving into a
// different shard's pool; proto getters are nil-safe, so callers can range a nil pool's instances.
func poolFor(status *operatorv1.StatusResponse, shardID string) *operatorv1.ShardPoolStatus {
	for _, pool := range status.GetPools() {
		if pool.GetShardId() == shardID {
			return pool
		}
	}
	return nil
}

// shardPods returns the pod names backing a shard. With instanceName empty,
// every pod of the pool is returned; otherwise only the matching instance's pod,
// using lenient matching (see instanceMatches) so "game 5"/"game5"/"game-5"/"5"
// all resolve to instance "game-5".
//
// A matched instance whose pod is not yet scheduled (empty PodName) is skipped —
// it has no logs to fetch — so callers that need to tell that case apart from
// "no instance matched" use findInstanceByName, which keeps the distinction.
func shardPods(status *operatorv1.StatusResponse, shardID, instanceName string) []string {
	var pods []string
	for _, inst := range poolFor(status, shardID).GetInstances() {
		if instanceName != "" && !instanceMatches(inst.GetName(), shardID, instanceName) {
			continue
		}
		if pod := inst.GetPodName(); pod != "" {
			pods = append(pods, pod)
		}
	}
	return pods
}

// findInstanceByName returns the pool instance matching instanceName (lenient
// matching, see instanceMatches), or nil when no instance of the shard matches.
// It mirrors shardPods' selection but does not require a running pod, so a caller
// can distinguish "instance not found" from "instance found but its pod isn't
// scheduled yet" — shardPods collapses both into an empty slice.
func findInstanceByName(
	status *operatorv1.StatusResponse,
	shardID, instanceName string,
) *operatorv1.ShardInstanceStatus {
	for _, inst := range poolFor(status, shardID).GetInstances() {
		if instanceMatches(inst.GetName(), shardID, instanceName) {
			return inst
		}
	}
	return nil
}

// normalizeInstanceKey reduces an instance identifier to a comparison key,
// dropping case and any separators/spaces so "game 5", "game5" and "GAME-5"
// reduce to the same key as "game-5".
func normalizeInstanceKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// instanceMatches reports whether the operator instance named instName satisfies
// a user-supplied reference for the given shard, tolerating separator/spacing/
// case variants. A pool's first instance is the bare shard ID (there is no
// "<shard>-1"), so it also matches an index-1 reference ("game 1"/"1"); a
// "<shard>-N" instance additionally matches the bare index "N".
func instanceMatches(instName, shardID, input string) bool {
	if input == "" {
		return false
	}
	if instName == input {
		return true
	}
	in := normalizeInstanceKey(input)
	normInst := normalizeInstanceKey(instName)
	normShard := normalizeInstanceKey(shardID)
	if in == normInst {
		return true
	}
	if instName == shardID {
		return in == "1" || in == normShard+"1"
	}
	if idx, ok := strings.CutPrefix(normInst, normShard); ok && idx != "" {
		return in == idx
	}
	return false
}

// shardInstanceNames lists a shard's operator instance names, for surfacing in
// "instance not found" errors.
func shardInstanceNames(status *operatorv1.StatusResponse, shardID string) []string {
	var names []string
	for _, inst := range poolFor(status, shardID).GetInstances() {
		names = append(names, inst.GetName())
	}
	return names
}

// errInstanceNotFound is the uniform "instance not found" error, listing the
// shard's live instances so the caller can correct the reference.
func errInstanceNotFound(status *operatorv1.StatusResponse, shardID, instanceName string) error {
	return eris.Errorf("instance %q not found for shard %q; available instances: %v",
		instanceName, shardID, shardInstanceNames(status, shardID))
}

// resolveInstanceName canonicalizes a user-supplied instance reference for a
// shard against the operator's live instances (lenient matching, scoped to the
// shard's own pool). Empty input defaults to the pool's first instance (the bare
// shard ID). On no match it returns an error listing the shard's instances.
func resolveInstanceName(ctx context.Context, operatorURL, shardID, instanceName string) (string, error) {
	instanceName = strings.TrimSpace(instanceName)
	if instanceName == "" {
		return shardID, nil
	}
	status, err := operatorStatus(ctx, operatorURL)
	if err != nil {
		return "", err
	}
	for _, inst := range poolFor(status, shardID).GetInstances() {
		if instanceMatches(inst.GetName(), shardID, instanceName) {
			return inst.GetName(), nil
		}
	}
	return "", errInstanceNotFound(status, shardID, instanceName)
}

// collectPodLogs returns up to tail lines of a shard pod's logs via the
// operator's StreamPodLogs RPC, non-following so the call terminates.
func collectPodLogs(ctx context.Context, operatorURL, podName string, tail int32) (string, error) {
	stream, err := newOperatorClient(
		operatorURL,
	).StreamPodLogs(ctx, connect.NewRequest(&operatorv1.StreamPodLogsRequest{
		PodName:   podName,
		TailLines: tail,
		Follow:    false,
	}))
	if err != nil {
		if eris.Is(err, context.Canceled) || eris.Is(err, context.DeadlineExceeded) {
			return "", nil
		}
		return "", eris.Wrapf(err, "stream logs for pod %s", podName)
	}
	defer func() { _ = stream.Close() }()

	var b strings.Builder
	for stream.Receive() {
		for _, line := range stream.Msg().GetLines() {
			b.WriteString(line.GetLine())
			b.WriteByte('\n')
		}
	}
	if err := stream.Err(); err != nil && !eris.Is(err, context.Canceled) && !eris.Is(err, context.DeadlineExceeded) {
		return "", eris.Wrapf(err, "log stream for pod %s", podName)
	}
	return b.String(), nil
}

// collectPlatformLogs returns up to tail lines of a platform component's logs
// (currently only "nats"). These pods live outside the operator's namespace,
// so logs come from the kube-apiserver via pkg/cluster. The stream follows the
// live pod, so it is bounded by platformLogWindow.
func collectPlatformLogs(ctx context.Context, component string, tail int32) (string, error) {
	ref, ok := platformPodRef(component)
	if !ok {
		return "", eris.Errorf("unknown platform component %q (want nats)", component)
	}

	out := make(chan cluster.LogLine, 256)
	errCh := make(chan error, 1)
	go func() {
		errCh <- clusterClient().StreamPlatformLogs(ctx, ref, cluster.LogsOpts{
			TailLines:    tail,
			FollowWindow: platformLogWindow,
		}, out)
	}()

	var b strings.Builder
	for line := range out {
		b.WriteString(line.Line)
		b.WriteByte('\n')
	}
	// The window expiring is the normal stop signal (ctx-cancel → nil); only
	// surface a genuine failure.
	if err := <-errCh; err != nil && !eris.Is(err, context.Canceled) && !eris.Is(err, context.DeadlineExceeded) {
		return "", eris.Wrapf(err, "stream %s logs", component)
	}
	return b.String(), nil
}

// platformPodRef resolves a component name to its pkg/cluster platform ref.
func platformPodRef(component string) (cluster.PlatformPodRef, bool) {
	for _, ref := range cluster.PlatformPods() {
		if ref.Name == component {
			return ref, true
		}
	}
	return cluster.PlatformPodRef{}, false
}

// clusterClient returns a process-wide cluster.Client. Reusing one Client lets
// its kube client (k3d kubeconfig + k8s clients) be built once and reused across
// tool calls instead of rebuilt every call; data is still read live each call.
//
//nolint:gochecknoglobals // intentional process-wide singleton cluster client
var clusterClient = sync.OnceValue(func() *cluster.Client {
	return cluster.NewClient(cluster.Defaults())
})

// deployedShards lists the cluster's deployed shards (with their world's
// organization/project) from the ShardPool CRs.
func deployedShards(ctx context.Context) ([]cluster.DeployedShard, error) {
	shards, err := clusterClient().DeployedShards(ctx)
	if err != nil {
		return nil, eris.Wrap(err, "failed to list deployed shards from the cluster")
	}
	return shards, nil
}

// resolveShardWorld returns the organization/project of a deployed shard,
// derived from the cluster so callers need no local world.toml. Explicit
// org+project (both set) short-circuit the lookup.
func resolveShardWorld(ctx context.Context, shardID, org, project string) (string, string, error) {
	if org != "" && project != "" {
		return org, project, nil
	}
	shards, err := deployedShards(ctx)
	if err != nil {
		return "", "", err
	}
	for _, s := range shards {
		if s.ShardID == shardID {
			return s.Organization, s.Project, nil
		}
	}
	available := make([]string, 0, len(shards))
	for _, s := range shards {
		available = append(available, s.ShardID)
	}
	return "", "", eris.Errorf("shard %q is not deployed on the cluster; deployed shards: %v", shardID, available)
}
