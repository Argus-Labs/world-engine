package cluster

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/rotisserie/eris"

	operatorv1 "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1"
	"github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1/operatorv1connect"
)

// LogLine is one rendered line from a shard pod, tagged with its source so
// the consumer can color / route per-shard or per-instance.
type LogLine struct {
	ShardID string // matches ShardPool.shardID (e.g. "gameplay", "lobby")
	// InstanceName is the operator's per-instance ID — same as ShardID for
	// pool_size=1 shards, "<shardID>" / "<shardID>-2" / … "<shardID>-N"
	// for pool_size=N. Use this (not ShardID) to label log lines so the
	// 10 pods of a "gameplay" pool render distinguishably.
	InstanceName string
	PodName      string // ephemeral; changes on rolling deploy
	// Timestamp is RFC3339Nano from the kubelet; empty if unavailable.
	Timestamp string
	Line      string
}

// LogsOpts controls StreamShardLogs.
type LogsOpts struct {
	// ShardIDs filters which shard pools to follow. Empty = all shards from
	// the operator's Status. Combine with InstanceNames for finer scope.
	ShardIDs []string
	// InstanceNames filters to specific pool instances (e.g. "gameplay-3").
	// Empty = every instance in the matching pools.
	InstanceNames []string
	// TailLines is how many historical lines to replay before tailing. 0 →
	// operator default (200). Capped server-side at 10000.
	TailLines int32
	// Previous fetches logs from the previous container incarnation. Useful
	// for inspecting a pod that just crashed.
	Previous bool
	// FollowWindow, if set, bounds how long StreamPlatformLogs follows a pod
	// once found. Zero means follow until ctx is done.
	FollowWindow time.Duration
}

// reconnectBackoff bounds how long we wait between pod-not-found retries
// (e.g. while a rolling deploy is in progress).
const reconnectBackoff = 2 * time.Second

// maxPreviousResubscribes bounds the re-subscribes a --previous dump may make
// after a transient cut. A live tail can retry forever because it has no end
// state to reach, but a previous-container dump does, and each retry costs
// accuracy: "previous" is resolved server-side per request, so if the pod
// restarts again during the backoff, the retry targets a NEWER incarnation and
// the output is silently spliced from two crashes. One retry covers a proxy RST
// mid-dump — the case worth recovering — while keeping that window small; past
// it the error is reported instead of retried, because a truncated dump the
// user is told about beats a seamless-looking one they cannot trust.
const maxPreviousResubscribes = 1

// StreamShardLogs opens a server-streaming log subscription against
// cardinal-operator for one or more shards/instances and sends every line
// into out. One goroutine per pod. Returns when ctx is canceled or every
// stream has terminated. Always closes out, including on error, so a caller
// ranging over it terminates.
//
// Pod identity is fetched from the operator's Status RPC. If a pod
// disappears mid-stream (rolling deploy, eviction), the goroutine refreshes
// Status and reconnects to the replacement pod. ctx cancellation stops all
// streams and is not an error.
//
// The returned error joins whatever each pod's stream ended on, so a caller
// can tell "the dump finished" from "it never started". Because it blocks, out
// must be drained concurrently — the same contract StreamPlatformLogs and
// logs.StreamFn already document.
func (c *Client) StreamShardLogs(ctx context.Context, opts LogsOpts, out chan<- LogLine) error {
	// Unconditional: a caller ranging over out would otherwise block forever
	// on the most common failure — an unreachable or unauthorized operator.
	defer close(out)

	rpc := c.operatorClient()

	instances, err := listShardInstances(ctx, rpc, opts.ShardIDs, opts.InstanceNames)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		// A filter the user asked for matching nothing is a bad selector, not
		// a quiet shard — `--shard gamepla` (typo) or `--shard gameplay-3`
		// (an instance id the output column shows, but `--shard` is
		// pool-scoped) would otherwise print "Tailing <env>" and exit 0
		// indistinguishably. Mirrors ResolveInstanceIDs / `world build`, which
		// fail the whole selection on unknown. The unfiltered path stays nil:
		// a scaled-to-zero env that genuinely has no running pods is not a
		// bad filter.
		if len(opts.ShardIDs) > 0 || len(opts.InstanceNames) > 0 {
			return eris.Errorf("no pods matched shard filter %v",
				append(slices.Clone(opts.ShardIDs), opts.InstanceNames...))
		}
		return nil
	}

	// One slot per instance rather than a shared error: the goroutines run
	// concurrently, and every pod's outcome is worth reporting, not just the
	// first or last to finish.
	errs := make([]error, len(instances))
	var wg sync.WaitGroup
	for i, inst := range instances {
		wg.Add(1)
		go func(idx int, in shardInstance) {
			defer wg.Done()
			errs[idx] = c.tailPod(ctx, rpc, in, opts, out)
		}(i, inst)
	}
	wg.Wait()
	return errors.Join(errs...)
}

type shardInstance struct {
	shardID      string
	instanceName string
	podName      string
}

// listShardInstances calls Status and flattens it into one entry per pod,
// optionally filtered by shard IDs and/or instance names.
func listShardInstances(
	ctx context.Context,
	rpc operatorv1connect.OperatorServiceClient,
	shardIDs, instanceNames []string,
) ([]shardInstance, error) {
	resp, err := rpc.Status(ctx, connect.NewRequest(&operatorv1.StatusRequest{}))
	if err != nil {
		return nil, eris.Wrap(err, "operator Status RPC")
	}

	wantShard := setOf(shardIDs)
	wantInstance := setOf(instanceNames)

	var out []shardInstance
	for _, pool := range resp.Msg.GetPools() {
		if len(wantShard) > 0 {
			if _, ok := wantShard[pool.GetShardId()]; !ok {
				continue
			}
		}
		for _, inst := range pool.GetInstances() {
			if inst.GetPodName() == "" {
				continue
			}
			if len(wantInstance) > 0 {
				if _, ok := wantInstance[inst.GetName()]; !ok {
					continue
				}
			}
			out = append(out, shardInstance{
				shardID:      pool.GetShardId(),
				instanceName: inst.GetName(),
				podName:      inst.GetPodName(),
			})
		}
	}
	return out, nil
}

func setOf(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(items))
	for _, s := range items {
		m[s] = struct{}{}
	}
	return m
}

// tailPod runs a single pod-log subscription, sending each batched line into
// out. A --previous query is finite: the operator forces follow off and
// closes the stream cleanly once the previous container's buffer is drained,
// so tailPod returns on that clean EOF — and also when the stream fails to
// open (no previous container) or the pod is gone, since a retry can't
// recover the targeted previous-container logs. A live tail re-subscribes on
// a clean EOF (a proxy idled the stream out) and on pod-not-found (the pod
// was rolled), so following ends only on ctx.
//
// Returns nil once the stream has run its course, and the terminal error
// otherwise. A --previous query that never opened is exactly as silent as an
// empty previous buffer, so reporting it is the only thing that tells the two
// apart. Cancellation is not an error: the caller asked for it.
func (c *Client) tailPod(
	ctx context.Context,
	rpc operatorv1connect.OperatorServiceClient,
	inst shardInstance,
	opts LogsOpts,
	out chan<- LogLine,
) error {
	current := inst.podName
	// since is a per-pod watermark: it dedupes the history the operator
	// replays when re-subscribing to the SAME pod (e.g. a proxy idle-cut).
	// It must NOT be carried across a pod rollover — the replacement pod
	// has its own history whose timestamps may predate the old pod's last
	// delivered line (a maxSurge>=1 rolling deploy boots the new pod
	// before the old one terminates). It is reset on the rollover branch
	// below so the new pod's startup lines are not filtered away.
	var since time.Time
	resubscribes := 0
	for {
		// A canceled ctx is the caller's own Ctrl+C or parent timeout, not a
		// stream failure, so it ends the tail without an error to report.
		if ctx.Err() != nil {
			return nil //nolint:nilerr // cancellation is the caller's doing, not a failure
		}
		var err error
		var delivered bool
		since, delivered, err = streamOne(ctx, rpc, inst.shardID, inst.instanceName, current, opts, out, since)
		if ctx.Err() != nil {
			return nil //nolint:nilerr // streamOne's error is just the cancellation surfacing
		}
		// A --previous query is a finite historical read, not a live tail: it
		// must not re-subscribe on a clean EOF or on an error before any line
		// arrived (the stream failed to open — e.g. no previous container — and
		// a retry would fail the same way) or the operator's not-found. Only a
		// transient cut AFTER some lines were delivered is worth a re-subscribe,
		// and that re-subscribe keeps Previous set so it finishes the dump
		// instead of silently switching to a live follow of the current
		// container. Live tails keep re-subscribing on any EOF/transient.
		//
		// opts is a value copy and Previous is never reassigned, so it still
		// describes what the user asked for on every pass. tail_lines=0 means
		// "operator default 200", so a re-subscribe always replays; `since` is
		// what actually suppresses the duplicates.
		switch {
		case opts.Previous && err == nil:
			return nil // dump finished cleanly (incl. an empty previous buffer)
		case opts.Previous && isNotFound(err):
			// Pod gone, or no previous container — k8s reports the latter as
			// NotFound on some versions. Either way --previous cannot recover it,
			// and saying so beats exiting 0 with nothing printed. Which of the two
			// it was turns on whether anything arrived first: reporting a dump the
			// pod cut off mid-flight as "no logs" would contradict the lines the
			// user just watched scroll past.
			if delivered {
				return eris.Wrapf(err, "previous-container log dump for pod %s was cut short", current)
			}
			return eris.Wrapf(err, "no previous-container logs for pod %s", current)
		case opts.Previous && !delivered:
			// The stream failed to open before any line, so a retry fails the
			// same way. Without this error the command prints nothing and exits
			// 0, indistinguishable from an empty previous buffer.
			return eris.Wrapf(err, "opening the previous-container log stream for pod %s", current)
		case opts.Previous && isTransient(err):
			if resubscribes >= maxPreviousResubscribes {
				return eris.Wrapf(err, "previous-container log dump for pod %s was cut short", current)
			}
			resubscribes++
		case err == nil, isTransient(err):
			// The stream ended without the pod finishing: proxies cut long-lived
			// streams (Cloudflare RSTs at ~100s idle). Re-subscribe to the same
			// pod; one that really is gone surfaces as not-found next time round.
		case isNotFound(err):
			if next, ok := findReplacementPod(ctx, rpc, inst.shardID, inst.instanceName, current); ok {
				current = next
				// Reset the watermark: the replacement pod has its own history
				// whose startup timestamps may predate the old pod's last
				// delivered line. Carrying the old watermark would silently
				// drop those lines via the !ts.After(since) filter in streamOne.
				since = time.Time{}
			}
		default:
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(reconnectBackoff):
		}
	}
}

// streamOne opens one StreamPodLogs subscription and pumps lines into out,
// skipping any at or before since so a re-subscribe does not re-deliver the
// replayed history. Returns the newest timestamp seen, whether the server ever
// delivered a message (false when the stream failed to open before any line),
// and nil when the server closes the stream cleanly.
func streamOne(
	ctx context.Context,
	rpc operatorv1connect.OperatorServiceClient,
	shardID, instanceName, podName string,
	opts LogsOpts,
	out chan<- LogLine,
	since time.Time,
) (time.Time, bool, error) {
	req := connect.NewRequest(&operatorv1.StreamPodLogsRequest{
		PodName:   podName,
		TailLines: opts.TailLines,
		Previous:  opts.Previous,
		Follow:    true, // world-cli `world logs` tails live; reconnects on pod churn.
	})
	stream, err := rpc.StreamPodLogs(ctx, req)
	if err != nil {
		return since, false, eris.Wrapf(err, "open StreamPodLogs for %s", podName)
	}
	defer func() { _ = stream.Close() }()

	latest := since
	var delivered bool
	for stream.Receive() {
		delivered = true
		for _, line := range stream.Msg().GetLines() {
			// An unparseable timestamp is delivered rather than dropped: it
			// cannot be ordered, and losing a line is worse than repeating one.
			if ts, tsErr := time.Parse(time.RFC3339Nano, line.GetTimestamp()); tsErr == nil {
				if !since.IsZero() && !ts.After(since) {
					continue
				}
				if ts.After(latest) {
					latest = ts
				}
			}
			select {
			case <-ctx.Done():
				return latest, delivered, ctx.Err()
			case out <- LogLine{
				ShardID:      shardID,
				InstanceName: instanceName,
				PodName:      podName,
				Timestamp:    line.GetTimestamp(),
				Line:         line.GetLine(),
			}:
			}
		}
	}
	return latest, delivered, stream.Err()
}

// findReplacementPod re-queries Status for a shard+instance pair and returns
// the current pod for that instance. Returns ok=false if no suitable pod is
// currently up (caller should back off and retry).
func findReplacementPod(
	ctx context.Context,
	rpc operatorv1connect.OperatorServiceClient,
	shardID, instanceName, lostPod string,
) (string, bool) {
	resp, err := rpc.Status(ctx, connect.NewRequest(&operatorv1.StatusRequest{}))
	if err != nil {
		return "", false
	}
	for _, pool := range resp.Msg.GetPools() {
		if pool.GetShardId() != shardID {
			continue
		}
		for _, inst := range pool.GetInstances() {
			if inst.GetName() != instanceName {
				continue
			}
			if inst.GetPodName() != "" && inst.GetPodName() != lostPod {
				return inst.GetPodName(), true
			}
		}
	}
	return "", false
}

// isNotFound checks whether an error from operator RPC indicates the pod is
// gone (vs a connection error or a server fault). The operator returns
// CodeNotFound on missing pods per the proto contract.
func isNotFound(err error) bool {
	var ce *connect.Error
	if eris.As(err, &ce) {
		return ce.Code() == connect.CodeNotFound
	}
	return false
}

// isTransient reports connection-level failures worth re-subscribing after.
// Cloudflare RSTs an idle stream at ~100s, which arrives as an HTTP/2
// INTERNAL_ERROR, not a clean EOF.
func isTransient(err error) bool {
	var ce *connect.Error
	if !eris.As(err, &ce) {
		return false
	}
	code := ce.Code()
	return code == connect.CodeInternal ||
		code == connect.CodeUnavailable ||
		code == connect.CodeDeadlineExceeded
}
