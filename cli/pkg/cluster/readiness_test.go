//nolint:testpackage // exercises unexported countReadyPoolPods/podRunsDesiredImage/podReady
package cluster

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// shardPod builds a shard pod with the operator's managed-by + shard-id labels
// (mirroring apps/cardinal-operator/internal/controller/pool_resources.go).
func shardPod(name, shardID, image string, ready bool) *corev1.Pod {
	p := &corev1.Pod{
		Name:      name,
		Namespace: operatorNamespace,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "cardinal-operator",
			shardIDLabel:                   shardID,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "shard", Image: image}},
		},
	}
	if ready {
		p.Status.Conditions = []corev1.PodCondition{
			{Type: corev1.PodReady, Status: corev1.ConditionTrue},
		}
	}
	return p
}

func TestPodRunsDesiredImage_MatchesDesiredRef(t *testing.T) {
	t.Parallel()
	desired := map[string]struct{}{"img:tag": {}}
	require.True(t, podRunsDesiredImage(shardPod("p", "s", "img:tag", true), desired))
}

func TestPodRunsDesiredImage_RejectsUndesiredRef(t *testing.T) {
	t.Parallel()
	desired := map[string]struct{}{"img:new": {}}
	require.False(t, podRunsDesiredImage(shardPod("p", "s", "img:old", true), desired))
}

func TestPodReady_TrueWhenConditionSet(t *testing.T) {
	t.Parallel()
	require.True(t, podReady(shardPod("p", "s", "img", true)))
	require.False(t, podReady(shardPod("p", "s", "img", false)))
}

// Bucketing by shard ID hides another shard's Ready pods — and an unlabeled
// operator pod — from a declared pool's count.
func TestCountReadyPoolPods_OnlyCountsDeclaredShards(t *testing.T) {
	t.Parallel()
	desired := map[string]struct{}{
		"img/game:L2":    {},
		"img/removed:L1": {},
	}
	unlabeled := shardPod("stray-0", "", "img/game:L2", true)
	delete(unlabeled.Labels, shardIDLabel)
	cs := k8sfake.NewSimpleClientset(
		shardPod("game-0", "game", "img/game:L2", true),
		shardPod("game-1", "game", "img/game:L2", true),
		shardPod("removed-0", "removed", "img/removed:L1", true), // Ready, desired — other shard
		shardPod("removed-1", "removed", "img/removed:L1", true),
		unlabeled,
	)

	game := []ShardPool{{ShardID: "game", PoolSize: 2}}
	require.Equal(t, 2, countReadyPoolPods(context.Background(), cs, game, desired),
		"only game's own Ready pods count toward game")

	removed := []ShardPool{{ShardID: "removed", PoolSize: 2}}
	require.Equal(t, 2, countReadyPoolPods(context.Background(), cs, removed, desired),
		"removed shard's pods still count for its own pool")
}

// An old-tag pod stays Ready for a beat during a rolling reload, so Ready alone
// isn't enough — it must also run a desired ref.
func TestCountReadyPoolPods_OnlyCountsReadyAndDesired(t *testing.T) {
	t.Parallel()
	pools := []ShardPool{{ShardID: "a", PoolSize: 3}}
	desired := map[string]struct{}{"img:new": {}}
	cs := k8sfake.NewSimpleClientset(
		shardPod("a-0-ready-new", "a", "img:new", true),    // counted
		shardPod("a-1-unready-new", "a", "img:new", false), // not Ready
		shardPod("a-2-ready-old", "a", "img:old", true),    // Ready but not desired
	)
	require.Equal(t, 1, countReadyPoolPods(context.Background(), cs, pools, desired))
}

// A list failure yields zero so the poll retries next tick.
func TestCountReadyPoolPods_TransientListErrorCountsZero(t *testing.T) {
	t.Parallel()
	pools := []ShardPool{{ShardID: "a", PoolSize: 1}}
	cs := k8sfake.NewSimpleClientset(shardPod("a-0", "a", "img:t", true))
	cs.PrependReactor("list", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver unreachable")
	})
	desired := map[string]struct{}{"img:t": {}}
	require.Equal(t, 0, countReadyPoolPods(context.Background(), cs, pools, desired))
}

// Regression test: the old cluster-wide count let a lingering removed pool's
// Ready pods satisfy the gate on tick 0, while game's own pods were still
// ContainerCreating.
func TestCountReadyPoolPods_RemovedShardPodsDoNotSatisfyGate(t *testing.T) {
	t.Parallel()
	pools := []ShardPool{{ShardID: "game", PoolSize: 2}}
	desired := map[string]struct{}{
		"img/game:L2":    {}, // freshly stamped tag on game
		"img/removed:L1": {}, // removed pool's CR still carries L1
	}
	cs := k8sfake.NewSimpleClientset(
		shardPod("game-0", "game", "img/game:L2", false), // fresh pods still booting
		shardPod("game-1", "game", "img/game:L2", false),
		shardPod("removed-0", "removed", "img/removed:L1", true), // the false-positive source
		shardPod("removed-1", "removed", "img/removed:L1", true),
	)
	require.Equal(t, 0, countReadyPoolPods(context.Background(), cs, pools, desired),
		"removed pool's Ready pods must not count toward game's gate")
}

// Happy path: every pool reaches PoolSize Ready desired pods.
func TestCountReadyPoolPods_AllPoolsReadySatisfiesGate(t *testing.T) {
	t.Parallel()
	pools := []ShardPool{{ShardID: "a", PoolSize: 2}, {ShardID: "b", PoolSize: 1}}
	desired := map[string]struct{}{"img:a": {}, "img:b": {}}
	cs := k8sfake.NewSimpleClientset(
		shardPod("a-0", "a", "img:a", true),
		shardPod("a-1", "a", "img:a", true),
		shardPod("b-0", "b", "img:b", true),
	)
	require.Equal(t, 3, countReadyPoolPods(context.Background(), cs, pools, desired))
}

// The gate waits for every pool, not just one.
func TestCountReadyPoolPods_PartialReadinessDoesNotSatisfyGate(t *testing.T) {
	t.Parallel()
	pools := []ShardPool{{ShardID: "a", PoolSize: 2}, {ShardID: "b", PoolSize: 2}}
	desired := map[string]struct{}{"img:a": {}, "img:b": {}}
	cs := k8sfake.NewSimpleClientset(
		shardPod("a-0", "a", "img:a", true),
		shardPod("a-1", "a", "img:a", true),
		shardPod("b-0", "b", "img:b", false), // b still booting
		shardPod("b-1", "b", "img:b", false),
	)
	require.Equal(t, 2, countReadyPoolPods(context.Background(), cs, pools, desired))
}

// The cap stops a surplus pool masking another pool's missing replicas.
func TestCountReadyPoolPods_CapsAtPoolSize(t *testing.T) {
	t.Parallel()
	pools := []ShardPool{{ShardID: "a", PoolSize: 2}, {ShardID: "b", PoolSize: 2}}
	desired := map[string]struct{}{"img:a": {}, "img:b": {}}
	cs := k8sfake.NewSimpleClientset(
		shardPod("a-0", "a", "img:a", true),
		shardPod("a-1", "a", "img:a", true),
		shardPod("a-2", "a", "img:a", true), // surplus
	)
	require.Equal(t, 2, countReadyPoolPods(context.Background(), cs, pools, desired),
		"a's surplus must be capped at its PoolSize")
}
