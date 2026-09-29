package cluster

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery/cached/memory"
	discoveryfake "k8s.io/client-go/discovery/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/restmapper"
)

// deferredMapperWithResources builds a *restmapper.DeferredDiscoveryRESTMapper
// (the concrete type kubeClient.mapper holds) backed by a fake discovery
// client reporting resources. Every applyOne/deleteShardPool/etc. call site
// funnels through mappingFor, so exercising it here covers them all.
func deferredMapperWithResources(
	t *testing.T, resources ...*metav1.APIResourceList,
) *restmapper.DeferredDiscoveryRESTMapper {
	t.Helper()
	fakeCS := k8sfake.NewSimpleClientset()
	fakeDisc, ok := fakeCS.Discovery().(*discoveryfake.FakeDiscovery)
	require.True(t, ok, "fake clientset's Discovery() must be *discoveryfake.FakeDiscovery")
	fakeDisc.Resources = resources
	return restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(fakeDisc))
}

func TestMappingFor_ResolvesRegisteredGVK(t *testing.T) {
	t.Parallel()

	mapper := deferredMapperWithResources(t, &metav1.APIResourceList{
		GroupVersion: shardPoolGVK.GroupVersion().String(),
		APIResources: []metav1.APIResource{
			{Name: "shardpools", Namespaced: true, Kind: shardPoolKind},
		},
	})
	k := &kubeClient{mapper: mapper}

	mapping, err := k.mappingFor(shardPoolGVK)
	require.NoError(t, err)
	require.Equal(t, "shardpools", mapping.Resource.Resource)
	require.Equal(t, meta.RESTScopeNameNamespace, mapping.Scope.Name())
}

func TestMappingFor_ErrorsWhenGVKRegisteredUnderAnotherGroup(t *testing.T) {
	t.Parallel()

	// Discovery knows about *a* group, just not ShardPool's — mappingFor's
	// reset+retry can't invent a mapping that was never there, and must
	// return the underlying RESTMapping error rather than panicking.
	mapper := deferredMapperWithResources(t, &metav1.APIResourceList{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{
			{Name: "deployments", Namespaced: true, Kind: "Deployment"},
		},
	})
	k := &kubeClient{mapper: mapper}

	_, err := k.mappingFor(shardPoolGVK)
	require.Error(t, err)
}
