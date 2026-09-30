package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func fakePod(name, ns string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels}}
}

func TestInstancePodSelector_ComposesLabelSelector(t *testing.T) {
	t.Parallel()
	require.Equal(t,
		shardPodSelector+","+instanceLabel+"=gameplay-2",
		instancePodSelector("gameplay-2"))
}

func TestWaitForPodsGone_SucceedsImmediatelyWhenNoMatchingPods(t *testing.T) {
	t.Parallel()
	cs := k8sfake.NewSimpleClientset()
	require.NoError(t, waitForPodsGone(context.Background(), cs, "cardinal", shardPodSelector, time.Second))
}

func TestWaitForPodsGone_TimesOutWithRemainingCount(t *testing.T) {
	t.Parallel()
	cs := k8sfake.NewSimpleClientset(
		fakePod("gameplay-0", "cardinal", map[string]string{"app.kubernetes.io/managed-by": "cardinal-operator"}),
	)
	err := waitForPodsGone(context.Background(), cs, "cardinal", shardPodSelector, 300*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "1 pod(s)")
	require.Contains(t, err.Error(), "still terminating")
}

// TestWaitForPodsGone_ScopesToInstanceSelector guards the property PurgeShardState
// depends on: a drain must not report "gone" while its own instance's pod is alive,
// nor be held up by a sibling replica's. Pods mirror the operator's labeling —
// pool-wide shard-id, per-replica instance (primary == pool id, rest numbered from 2).
func TestWaitForPodsGone_ScopesToInstanceSelector(t *testing.T) {
	t.Parallel()
	pod := func(instance string) *corev1.Pod {
		return fakePod(instance+"-aaa", "cardinal", map[string]string{
			"app.kubernetes.io/managed-by": "cardinal-operator",
			"cardinal.argus.gg/shard-id":   "gameplay",
			instanceLabel:                  instance,
		})
	}
	cs := k8sfake.NewSimpleClientset(pod("gameplay"), pod("gameplay-2"), pod("gameplay-3"))

	// An unseeded instance drains immediately despite three live siblings.
	require.NoError(t,
		waitForPodsGone(context.Background(), cs, "cardinal", instancePodSelector("gameplay-4"), time.Second))

	// Each live instance blocks on its own pod only. The prior shard-id selector
	// matched zero pods for "gameplay-2" and silently no-oped.
	for _, instance := range []string{"gameplay", "gameplay-2"} {
		err := waitForPodsGone(
			context.Background(), cs, "cardinal", instancePodSelector(instance), 300*time.Millisecond)
		require.Error(t, err)
		require.Contains(t, err.Error(), "1 pod(s)")
	}
}

func TestWaitForPodsGone_SurfacesPersistentListError(t *testing.T) {
	t.Parallel()
	cs := k8sfake.NewSimpleClientset()
	cs.PrependReactor("list", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver unreachable")
	})

	err := waitForPodsGone(context.Background(), cs, "cardinal", shardPodSelector, 300*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "apiserver unreachable")
	require.Contains(t, err.Error(), "list pods")
}
