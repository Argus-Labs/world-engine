package cluster

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rotisserie/eris"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// fieldManager identifies our process to the K8s apiserver for Server-Side
// Apply. Constant so re-applies don't conflict with themselves.
const fieldManager = "world-cli/cluster"

// kubeClient bundles the discovery + dynamic clients needed to apply
// arbitrary unstructured manifests. Constructed once per Client.Start call
// (kubeconfig from k3d). restCfg is kept around so port-forward consumers
// can reuse it without re-parsing the kubeconfig.
type kubeClient struct {
	restCfg *rest.Config
	dynamic dynamic.Interface
	mapper  *restmapper.DeferredDiscoveryRESTMapper
}

// newKubeClient builds a kubeClient from a kubeconfig YAML blob (as returned
// by `k3d kubeconfig get`).
func newKubeClient(kubeconfig []byte) (*kubeClient, error) {
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, eris.Wrap(err, "parse kubeconfig")
	}

	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, eris.Wrap(err, "dynamic client")
	}

	dc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, eris.Wrap(err, "discovery client")
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	return &kubeClient{restCfg: restCfg, dynamic: dyn, mapper: mapper}, nil
}

// applyYAML server-side-applies every document in a multi-document YAML.
// Namespaces in the YAML are honored; documents without a namespace target
// cluster-scoped resources (CRDs, Namespaces).
func (k *kubeClient) applyYAML(ctx context.Context, doc []byte) error {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(doc), 4096)
	for {
		obj := &unstructured.Unstructured{}
		if err := dec.Decode(obj); err != nil {
			if eris.Is(err, io.EOF) {
				return nil
			}
			return eris.Wrap(err, "decode yaml")
		}
		if len(obj.Object) == 0 {
			continue // skip empty documents between separators
		}
		if err := k.applyOne(ctx, obj); err != nil {
			return err
		}
	}
}

// mappingFor resolves gvk's REST mapping, resetting the cached discovery
// mapper and retrying once on failure — a CRD applied earlier in the same
// run (or the ShardPool CRD on a freshly created cluster) may not yet be
// visible to a stale cache.
func (k *kubeClient) mappingFor(gvk schema.GroupVersionKind) (*meta.RESTMapping, error) {
	mapping, err := k.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		k.mapper.Reset()
		mapping, err = k.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	}
	return mapping, err
}

// applyOne server-side-applies a single resource.
func (k *kubeClient) applyOne(ctx context.Context, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()
	mapping, err := k.mappingFor(gvk)
	if err != nil {
		return eris.Wrapf(err, "rest mapping for %s/%s", gvk.Kind, obj.GetName())
	}

	data, err := obj.MarshalJSON()
	if err != nil {
		return eris.Wrap(err, "marshal object")
	}

	var iface dynamic.ResourceInterface
	if mapping.Scope.Name() == "namespace" {
		ns := obj.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		iface = k.dynamic.Resource(mapping.Resource).Namespace(ns)
	} else {
		iface = k.dynamic.Resource(mapping.Resource)
	}

	// Force=true means we override field-ownership conflicts (e.g. with a
	// prior `kubectl apply`). Idempotent re-apply from world-cli should
	// always win against ad-hoc client-side-apply.
	force := true
	_, err = iface.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: fieldManager,
		Force:        &force,
	})
	if err != nil {
		return eris.Wrapf(err, "apply %s/%s", gvk.Kind, obj.GetName())
	}
	return nil
}

// shardPoolGVK is the ShardPool CR's GroupVersionKind, built from the shared
// identity constants (shardpool.go) so it stays in lockstep with shardPoolYAML.
var shardPoolGVK = schema.GroupVersionKind{Group: shardPoolGroup, Version: shardPoolVersion, Kind: shardPoolKind}

// deleteShardPoolsInNamespace deletes every ShardPool CR in namespace in one call;
// k8s GCs each pool's Deployments/pods via owner references. Idempotent.
//
// namespace MUST be non-empty: the dynamic client treats Namespace("") as "all
// namespaces", so a blank value would wipe every world's pools — hence the guard.
func (k *kubeClient) deleteShardPoolsInNamespace(ctx context.Context, namespace string) error {
	if namespace == "" {
		return eris.New("deleteShardPoolsInNamespace: namespace is required")
	}

	mapping, err := k.mappingFor(shardPoolGVK)
	if err != nil {
		return eris.Wrap(err, "rest mapping for ShardPool")
	}

	err = k.dynamic.Resource(mapping.Resource).Namespace(namespace).
		DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return eris.Wrapf(err, "delete ShardPools in namespace %s", namespace)
	}
	return nil
}

// deleteShardPool deletes a single ShardPool CR by name — the CR's metadata.name
// is always set to its shard ID (shardPoolYAML), so callers pass a shard ID
// directly. k8s GCs its owned Deployment/pods via owner references; every other
// ShardPool in namespace is untouched. Idempotent — a missing CR is a no-op.
func (k *kubeClient) deleteShardPool(ctx context.Context, namespace, shardID string) error {
	if namespace == "" {
		return eris.New("deleteShardPool: namespace is required")
	}

	mapping, err := k.mappingFor(shardPoolGVK)
	if err != nil {
		return eris.Wrap(err, "rest mapping for ShardPool")
	}

	err = k.dynamic.Resource(mapping.Resource).Namespace(namespace).
		Delete(ctx, shardID, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return eris.Wrapf(err, "delete ShardPool/%s in namespace %s", shardID, namespace)
	}
	return nil
}

// operatorDeploymentName is the operator Deployment's metadata.name in the embedded
// manifest (manifests/operator/operator.yaml). Deleting it stops the operator pod;
// its Service/RBAC are left for ensureOperator to re-apply on the next world start.
const operatorDeploymentName = "controller-manager"

// deleteOperatorDeployment removes the operator Deployment from namespace,
// stopping the operator pod. Idempotent — a missing Deployment is a no-op.
func (k *kubeClient) deleteOperatorDeployment(ctx context.Context, namespace string) error {
	if namespace == "" {
		return eris.New("deleteOperatorDeployment: namespace is required")
	}
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	mapping, err := k.mappingFor(gvk)
	if err != nil {
		return eris.Wrap(err, "rest mapping for Deployment")
	}
	err = k.dynamic.Resource(mapping.Resource).Namespace(namespace).
		Delete(ctx, operatorDeploymentName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return eris.Wrapf(err, "delete operator Deployment in namespace %s", namespace)
	}
	return nil
}

// listShardPoolWorldKeys returns the world identity keys (WorldKey, "{org}/{project}")
// of every ShardPool CR in namespace. Reads the CRs straight from the apiserver, so
// it doesn't need the operator running (which only exists while a world is deployed).
// A missing CRD (freshly created cluster) yields an empty set.
func (k *kubeClient) listShardPoolWorldKeys(ctx context.Context, namespace string) (map[string]struct{}, error) {
	mapping, err := k.mappingFor(shardPoolGVK)
	if err != nil {
		return nil, eris.Wrap(err, "rest mapping for ShardPool")
	}
	list, err := k.dynamic.Resource(mapping.Resource).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]struct{}{}, nil
		}
		return nil, eris.Wrapf(err, "list ShardPools in namespace %s", namespace)
	}
	out := make(map[string]struct{}, len(list.Items))
	for i := range list.Items {
		org, _, _ := unstructured.NestedString(list.Items[i].Object, "spec", "organization")
		proj, _, _ := unstructured.NestedString(list.Items[i].Object, "spec", "project")
		if org == "" && proj == "" {
			continue
		}
		out[WorldKey(org, proj)] = struct{}{}
	}
	return out, nil
}

// shardPoolImages returns the set of desired "image:tag" refs across every
// ShardPool CR in namespace. The operator's Deploy RPC writes spec.imageTag, and
// each shard pod runs "<spec.image>:<spec.imageTag>", so a pod is "the current
// deployment" only if its container image is in this set. WaitForShardsReady uses
// that to ignore a rolling Reload's still-Ready old-tag pods and wait for the
// freshly-rolled ones. A missing CRD (fresh cluster) yields an empty set.
func (k *kubeClient) shardPoolImages(ctx context.Context, namespace string) (map[string]struct{}, error) {
	mapping, err := k.mappingFor(shardPoolGVK)
	if err != nil {
		return nil, eris.Wrap(err, "rest mapping for ShardPool")
	}
	list, err := k.dynamic.Resource(mapping.Resource).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]struct{}{}, nil
		}
		return nil, eris.Wrapf(err, "list ShardPools in namespace %s", namespace)
	}
	out := make(map[string]struct{}, len(list.Items))
	for i := range list.Items {
		image, _, _ := unstructured.NestedString(list.Items[i].Object, "spec", "image")
		tag, _, _ := unstructured.NestedString(list.Items[i].Object, "spec", "imageTag")
		if image == "" || tag == "" {
			continue // no tag deployed yet — nothing to match against
		}
		out[image+":"+tag] = struct{}{}
	}
	return out, nil
}

// gcOrphanedShardPools deletes ShardPool CRs of cfg's world whose shard ID is no
// longer in world.toml — applyShardPools never prunes, so a dropped shard keeps
// running otherwise. Org/project-scoped so another world's CRs are safe; a CR's
// name is its shard ID (shardPoolYAML). Delete failures are logged, not blocking.
func gcOrphanedShardPools(ctx context.Context, k *kubeClient, cfg toml.Config) error {
	mapping, err := k.mappingFor(shardPoolGVK)
	if err != nil {
		return eris.Wrap(err, "rest mapping for ShardPool")
	}
	ri := k.dynamic.Resource(mapping.Resource).Namespace(operatorNamespace)
	list, err := ri.List(ctx, metav1.ListOptions{})
	if err != nil {
		return eris.Wrapf(err, "list ShardPools in namespace %s", operatorNamespace)
	}

	declared := make(map[string]struct{}, len(cfg.Shards))
	for _, s := range cfg.Shards {
		declared[s.ID] = struct{}{}
	}
	world := WorldKey(cfg.Organization, cfg.Project)

	for i := range list.Items {
		obj := list.Items[i].Object
		shardID, _, _ := unstructured.NestedString(obj, "spec", "shardID")
		org, _, _ := unstructured.NestedString(obj, "spec", "organization")
		proj, _, _ := unstructured.NestedString(obj, "spec", "project")
		if shardID == "" || WorldKey(org, proj) != world {
			continue
		}
		if _, ok := declared[shardID]; ok {
			continue
		}
		if err := ri.Delete(ctx, shardID, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			fmt.Fprintf(os.Stderr, "world-cli: delete orphaned ShardPool %s/%s: %v\n", operatorNamespace, shardID, err)
		}
	}
	return nil
}

// listDeployedShards reads ShardPool CRs and returns each shard with its owning
// world's organization/project.
func (k *kubeClient) listDeployedShards(ctx context.Context, namespace string) ([]DeployedShard, error) {
	mapping, err := k.mapper.RESTMapping(shardPoolGVK.GroupKind(), shardPoolGVK.Version)
	if err != nil {
		k.mapper.Reset()
		mapping, err = k.mapper.RESTMapping(shardPoolGVK.GroupKind(), shardPoolGVK.Version)
		if err != nil {
			return nil, eris.Wrap(err, "rest mapping for ShardPool")
		}
	}
	list, err := k.dynamic.Resource(mapping.Resource).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, eris.Wrapf(err, "list ShardPools in namespace %s", namespace)
	}
	out := make([]DeployedShard, 0, len(list.Items))
	for i := range list.Items {
		obj := list.Items[i].Object
		shardID, _, _ := unstructured.NestedString(obj, "spec", "shardID")
		if shardID == "" {
			continue
		}
		org, _, _ := unstructured.NestedString(obj, "spec", "organization")
		proj, _, _ := unstructured.NestedString(obj, "spec", "project")
		out = append(out, DeployedShard{ShardID: shardID, Organization: org, Project: proj})
	}
	return out, nil
}

// waitForCRD blocks until the named CRD reports Established=True or ctx
// expires. Polls every 500ms.
func (k *kubeClient) waitForCRD(ctx context.Context, name string) error {
	gvr := apiextensionsv1.SchemeGroupVersion.WithResource("customresourcedefinitions")
	err := wait.PollUntilContextCancel(ctx, 500*time.Millisecond, true,
		func(ctx context.Context) (bool, error) {
			obj, err := k.dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil // CRD not registered yet — keep polling
			}
			if err != nil {
				return false, eris.Wrapf(err, "get CRD %s", name)
			}
			return crdEstablished(obj), nil
		})
	// A cancelled/expired context is the timeout case; anything else is the
	// condition's own (already-wrapped) error surfaced verbatim.
	if wait.Interrupted(err) {
		return eris.Wrapf(ctx.Err(), "wait for CRD %s established", name)
	}
	return err
}

// crdEstablished reports whether a CRD object's status has a True
// "Established" condition.
func crdEstablished(obj *unstructured.Unstructured) bool {
	conds, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found || err != nil {
		return false
	}
	for _, raw := range conds {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if c["type"] == "Established" && c["status"] == "True" {
			return true
		}
	}
	return false
}
