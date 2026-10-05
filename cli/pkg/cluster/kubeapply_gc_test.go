//nolint:testpackage // exercises unexported gcOrphanedShardPools/kubeClient helpers
package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// shardPoolObj builds an unstructured ShardPool CR. metadata.name == shardID,
// mirroring shardPoolYAML.
func shardPoolObj(shardID, org, project string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": shardPoolGroup + "/" + shardPoolVersion,
		"kind":       shardPoolKind,
		"metadata": map[string]any{
			"name":      shardID,
			"namespace": operatorNamespace,
		},
		"spec": map[string]any{
			"shardID":      shardID,
			"organization": org,
			"project":      project,
			"image":        "img/" + shardID,
			"imageTag":     "L1",
			"poolSize":     int64(2),
		},
	}}
}

// dynamicShardPoolClient builds a kubeClient holding the given ShardPool CRs,
// following the services_test.go pattern so List/Delete hit the in-memory objects.
func dynamicShardPoolClient(t *testing.T, objs ...*unstructured.Unstructured) *kubeClient {
	t.Helper()
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: shardPoolGroup, Version: shardPoolVersion, Resource: "shardpools"}: "ShardPoolList",
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, toObjects(objs)...)
	mapper := deferredMapperWithResources(t, &metav1.APIResourceList{
		GroupVersion: shardPoolGVK.GroupVersion().String(),
		APIResources: []metav1.APIResource{
			{Name: "shardpools", Namespaced: true, Kind: shardPoolKind},
		},
	})
	return &kubeClient{dynamic: dyn, mapper: mapper}
}

func toObjects(objs []*unstructured.Unstructured) []runtime.Object {
	out := make([]runtime.Object, 0, len(objs))
	for _, o := range objs {
		out = append(out, o)
	}
	return out
}

func TestGcOrphanedShardPools_DeletesUndeclaredKeepsDeclaredAndOtherWorlds(t *testing.T) {
	t.Parallel()

	const org, proj = "argus", "rampage"
	k := dynamicShardPoolClient(t,
		shardPoolObj("game", org, proj),              // declared in cfg → kept
		shardPoolObj("lobby", org, proj),             // declared in cfg → kept
		shardPoolObj("removed", org, proj),           // dropped from cfg, same world → reaped
		shardPoolObj("otherworld", "other", "world"), // different world → untouched (defensive scoping)
	)
	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[shards]]
		id = "game"
		[[shards]]
		id = "lobby"
	`)
	require.NoError(t, gcOrphanedShardPools(context.Background(), k, cfg))

	get := func(name string) error {
		gvr := schema.GroupVersionResource{Group: shardPoolGroup, Version: shardPoolVersion, Resource: "shardpools"}
		_, err := k.dynamic.Resource(gvr).
			Namespace(operatorNamespace).
			Get(context.Background(), name, metav1.GetOptions{})
		return err
	}
	require.NoError(t, get("game"), "declared shard must be kept")
	require.NoError(t, get("lobby"), "declared shard must be kept")
	require.True(t, apierrors.IsNotFound(get("removed")), "dropped same-world shard must be reaped")
	require.NoError(t, get("otherworld"), "a different world's shard must be left untouched")
}

// Nothing to reap: an empty namespace is a no-op, not an error.
func TestGcOrphanedShardPools_NoopWhenNoPoolsExist(t *testing.T) {
	t.Parallel()
	k := dynamicShardPoolClient(t) // no objects
	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[shards]]
		id = "game"
	`)
	require.NoError(t, gcOrphanedShardPools(context.Background(), k, cfg))
}

// cfg matching the cluster exactly must delete nothing.
func TestGcOrphanedShardPools_NoOpWhenAllDeclared(t *testing.T) {
	t.Parallel()
	const org, proj = "argus", "rampage"
	k := dynamicShardPoolClient(t,
		shardPoolObj("game", org, proj),
		shardPoolObj("lobby", org, proj),
	)
	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[shards]]
		id = "game"
		[[shards]]
		id = "lobby"
	`)
	require.NoError(t, gcOrphanedShardPools(context.Background(), k, cfg))

	gvr := schema.GroupVersionResource{Group: shardPoolGroup, Version: shardPoolVersion, Resource: "shardpools"}
	list, err := k.dynamic.Resource(gvr).Namespace(operatorNamespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, 2, "no CRs should be deleted when all are declared")
}

// Declaring one shard reaps every other same-world CR; a different world's
// CR survives.
func TestGcOrphanedShardPools_ReapsEveryUndeclaredSameWorldShard(t *testing.T) {
	t.Parallel()
	const org, proj = "argus", "rampage"
	k := dynamicShardPoolClient(t,
		shardPoolObj("game", org, proj),              // declared → kept
		shardPoolObj("lobby", org, proj),             // dropped, same world → reaped
		shardPoolObj("meta", org, proj),              // dropped, same world → reaped
		shardPoolObj("otherworld", "other", "world"), // different world → untouched
	)
	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[shards]]
		id = "game"
	`)
	require.NoError(t, gcOrphanedShardPools(context.Background(), k, cfg))

	gvr := schema.GroupVersionResource{Group: shardPoolGroup, Version: shardPoolVersion, Resource: "shardpools"}
	get := func(name string) error {
		_, err := k.dynamic.Resource(gvr).
			Namespace(operatorNamespace).
			Get(context.Background(), name, metav1.GetOptions{})
		return err
	}
	require.NoError(t, get("game"), "declared shard kept")
	require.True(t, apierrors.IsNotFound(get("lobby")), "undeclared same-world shard reaped")
	require.True(t, apierrors.IsNotFound(get("meta")), "undeclared same-world shard reaped")
	require.NoError(t, get("otherworld"), "other-world shard untouched")
}
