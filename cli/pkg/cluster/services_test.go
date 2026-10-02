package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/yaml"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// envMap collapses an EnvVar slice into name→value for assertion convenience.
func envMap(env []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		m[e.Name] = e.Value
	}
	return m
}

func TestServiceEnv_SeedsCardinalIdentity(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "meta"
		path = "."
	`)
	env := envMap(serviceEnv(cfg, cfg.Services[0]))
	require.Equal(t, natsClusterURL, env["NATS_URL"])
	require.Equal(t, DefaultRegion, env["CARDINAL_REGION"])
	// Full wire identity so a service like meta subscribes on the right subject.
	require.Equal(t, "argus", env["CARDINAL_ORG"])
	require.Equal(t, "rampage", env["CARDINAL_PROJECT"])
	require.Equal(t, "meta", env["CARDINAL_SHARD_ID"])
	require.NotContains(t, env, dbDSNEnvVar)
}

func TestServiceEnv_SvcEnvOverridesSeeds(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "gameplay"
		path = "."
		env = { NATS_URL = "nats://custom:4222", CARDINAL_REGION = "us-west1", FOO = "bar" }
	`)
	env := envMap(serviceEnv(cfg, cfg.Services[0]))
	require.Equal(t, "nats://custom:4222", env["NATS_URL"])
	require.Equal(t, "us-west1", env["CARDINAL_REGION"])
	require.Equal(t, "bar", env["FOO"])
}

func TestServiceEnv_DBDSNOnlyWhenDBTrueAndAbsent(t *testing.T) {
	t.Parallel()

	t.Run("injected when db=true and absent", func(t *testing.T) {
		t.Parallel()
		cfg := mustLoad(t, `
			organization = "argus"
			project = "rampage"
			[[services]]
			id = "gameplay"
			path = "."
			db = true
		`)
		env := envMap(serviceEnv(cfg, cfg.Services[0]))
		require.Equal(t, projectDBDSN(cfg), env[dbDSNEnvVar])
		require.NotEmpty(t, env[dbDSNEnvVar])
	})

	t.Run("world.toml DB_DSN wins", func(t *testing.T) {
		t.Parallel()
		cfg := mustLoad(t, `
			organization = "argus"
			project = "rampage"
			[[services]]
			id = "gameplay"
			path = "."
			db = true
			env = { DB_DSN = "postgres://explicit" }
		`)
		env := envMap(serviceEnv(cfg, cfg.Services[0]))
		require.Equal(t, "postgres://explicit", env[dbDSNEnvVar])
	})

	t.Run("no DB_DSN when db=false", func(t *testing.T) {
		t.Parallel()
		cfg := mustLoad(t, `
			organization = "argus"
			project = "rampage"
			[[services]]
			id = "gameplay"
			path = "."
		`)
		env := envMap(serviceEnv(cfg, cfg.Services[0]))
		require.NotContains(t, env, dbDSNEnvVar)
	})
}

func TestServiceEnv_ConfigDBSetsPostgresVars(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "postgres"
		image = "postgres:16"
		config_db = true
		env = { POSTGRES_USER = "ignored" }
	`)
	env := envMap(serviceEnv(cfg, cfg.Services[0]))
	// world-cli's config_db credentials always win over svc.Env.
	require.Equal(t, configDBUser, env["POSTGRES_USER"])
	require.Equal(t, configDBPassword, env["POSTGRES_PASSWORD"])
	require.Equal(t, "rampage", env["POSTGRES_DB"])
}

func TestServiceEnv_SortedDeterministic(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "gameplay"
		path = "."
		env = { ZED = "1", ABC = "2" }
	`)
	env := serviceEnv(cfg, cfg.Services[0])
	for i := 1; i < len(env); i++ {
		require.Less(t, env[i-1].Name, env[i].Name, "env must be sorted by name")
	}
}

func TestServiceSourceImage(t *testing.T) {
	t.Parallel()
	// Matches docker's GameServiceFromConfig (Image == container name) :latest.
	require.Equal(t, "rampage-meta-service:latest", serviceSourceImage("rampage", "meta"))
}

// TestDeployServices_PathKindSelectionAndImages exercises the pure parts of the
// DeployServices loop (path-kind filtering + source/dest image strings) without
// a live cluster. Image-kind entries are skipped; path-kind entries map to the
// build's local :latest source and the registry-prefixed dest the import uses.
func TestDeployServices_PathKindSelectionAndImages(t *testing.T) {
	t.Parallel()

	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "meta"
		path = "services/meta/cmd"
		[[services]]
		id = "postgres"
		image = "postgres:16"
	`)
	c := NewClient(Config{})

	var pathKind []toml.GameService
	for _, svc := range cfg.Services {
		if svc.IsBuiltFromSource() {
			pathKind = append(pathKind, svc)
		}
	}

	require.Len(t, pathKind, 1, "only the path-kind service is selected")
	svc := pathKind[0]
	require.Equal(t, "meta", svc.ID)
	require.Equal(t, "rampage-meta-service:latest", serviceSourceImage(cfg.Project, svc.ID))
	require.Equal(t,
		"k3d-world-engine-registry.localhost:5000/rampage/meta",
		c.imageRef(cfg.Project, svc.ID),
		"dest ref (pre-tag) targets the local k3d registry")
}

func TestGcOrphanedServices_DeletesUndeclaredKeepsDeclared(t *testing.T) {
	t.Parallel()

	ns := projectServiceNamespace()
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		{Group: "", Version: "v1", Resource: "services"}:        "ServiceList",
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind,
		managedServiceObj("apps/v1", "Deployment", "rampage-meta-service", ns, "rampage"),
		managedServiceObj("apps/v1", "Deployment", "rampage-stale-service", ns, "rampage"),
		managedServiceObj("v1", "Service", "rampage-stale-service", ns, "rampage"),
		// A Service whose Deployment is already gone — must still be reaped.
		managedServiceObj("v1", "Service", "rampage-ghost-service", ns, "rampage"),
	)
	k := &kubeClient{dynamic: dyn}

	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "meta"
		path = "."
	`)
	require.NoError(t, gcOrphanedServices(context.Background(), k, cfg))

	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	svcGVR := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	get := func(gvr schema.GroupVersionResource, name string) error {
		_, err := dyn.Resource(gvr).Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
		return err
	}

	// The undeclared (orphan) Deployment + its Service are reaped.
	require.True(
		t,
		apierrors.IsNotFound(get(deployGVR, "rampage-stale-service")),
		"orphan deployment should be deleted",
	)
	require.True(t, apierrors.IsNotFound(get(svcGVR, "rampage-stale-service")), "orphan service should be deleted")
	// A Service with no surviving Deployment is still reaped via the Service list.
	require.True(t, apierrors.IsNotFound(get(svcGVR, "rampage-ghost-service")), "ghost service should be reaped")
	// The declared service is left untouched.
	require.NoError(t, get(deployGVR, "rampage-meta-service"), "declared service must be kept")
}

// managedServiceObj builds an unstructured Deployment/Service carrying the
// world-cli-managed + project labels gcOrphanedServices selects on.
func managedServiceObj(apiVersion, kind, name, ns, project string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
			"labels": map[string]any{
				"app":          name,
				managedByLabel: "true",
				projectLabel:   project,
			},
		},
	}}
}

// newApplyRecordingKubeClient builds a kubeClient backed by a fake dynamic
// client that records every Server-Side Apply (Patch with ApplyPatchType) into
// the returned map, keyed "Deployment/{name}" or "Service/{name}" -> raw patch
// bytes. The fake tracker's Apply requires the object to already exist, so the
// reactor captures the apply intent rather than the cluster state.
func newApplyRecordingKubeClient(t *testing.T) (*kubeClient, map[string][]byte) {
	t.Helper()
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		{Group: "", Version: "v1", Resource: "services"}:        "ServiceList",
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind)

	applied := make(map[string][]byte)
	dyn.PrependReactor("patch", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		pa, ok := action.(k8stesting.PatchActionImpl)
		if !ok || pa.GetPatchType() != types.ApplyPatchType {
			return false, nil, nil
		}
		gvr := action.GetResource()
		kind := "Service"
		if gvr.Group == "apps" {
			kind = "Deployment"
		}
		applied[kind+"/"+pa.GetName()] = pa.GetPatch()
		return true, nil, nil
	})

	mapper := deferredMapperWithResources(t,
		&metav1.APIResourceList{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{{Name: "deployments", Namespaced: true, Kind: "Deployment"}},
		},
		&metav1.APIResourceList{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{{Name: "services", Namespaced: true, Kind: "Service"}},
		},
	)
	return &kubeClient{dynamic: dyn, mapper: mapper}, applied
}

// appliedService unmarshals a captured Apply patch into a corev1.Service so its
// type/ports can be asserted.
func appliedService(t *testing.T, patch []byte) corev1.Service {
	t.Helper()
	var svc corev1.Service
	require.NoError(t, yaml.Unmarshal(patch, &svc))
	return svc
}

// TestEnsureServices_ConfigDBZeroPorts_CreatesService is the regression test for
// the bug where a config_db service with no declared ports got a Deployment but
// no Service, leaving its DSN host (a Service DNS name) unresolvable so every
// DB consumer crash-looped. After the fix a Service is always created for
// config_db, synthesizing the Postgres port (5432) when none is declared.
func TestEnsureServices_ConfigDBZeroPorts_CreatesService(t *testing.T) {
	t.Parallel()

	k, applied := newApplyRecordingKubeClient(t)
	c := NewClient(Config{})

	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "postgres"
		image = "postgres:16"
		config_db = true
	`)
	require.NoError(t, c.ensureServices(context.Background(), k, cfg))

	name := serviceContainerName(cfg.Project, cfg.Services[0].ID)
	require.Contains(t, applied, "Deployment/"+name,
		"Deployment should be applied for a 0-port config_db")
	require.Contains(t, applied, "Service/"+name,
		"Service should be applied for a 0-port config_db (the fix)")

	// The synthesized Service must expose the Postgres port (5432) and be
	// ClusterIP so in-cluster consumers reach it via the DSN host that
	// projectDBDSN builds.
	svc := appliedService(t, applied["Service/"+name])
	require.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	require.Len(t, svc.Spec.Ports, 1)
	require.Equal(t, projectDBPort, svc.Spec.Ports[0].Port)
	require.Equal(t, int(projectDBPort), svc.Spec.Ports[0].TargetPort.IntValue())
}

// TestEnsureServices_NonConfigDBZeroPorts_NoService locks the existing behavior
// that a plain (non-config_db) service with no declared ports gets a Deployment
// but no Service — the fix must not change this, only config_db synthesizes a
// port.
func TestEnsureServices_NonConfigDBZeroPorts_NoService(t *testing.T) {
	t.Parallel()

	k, applied := newApplyRecordingKubeClient(t)
	c := NewClient(Config{})

	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "meta"
		image = "postgres:16"
	`)
	require.NoError(t, c.ensureServices(context.Background(), k, cfg))

	name := serviceContainerName(cfg.Project, cfg.Services[0].ID)
	require.Contains(t, applied, "Deployment/"+name,
		"Deployment should be applied for a 0-port non-config_db service")
	require.NotContains(t, applied, "Service/"+name,
		"Service should NOT be applied for a 0-port non-config_db service")
}

// TestEnsureServices_ConfigDBWithPorts_CreatesService is the contrast case: a
// config_db that declares its port still gets both a Deployment and a Service,
// and the fix must not alter the declared port.
func TestEnsureServices_ConfigDBWithPorts_CreatesService(t *testing.T) {
	t.Parallel()

	k, applied := newApplyRecordingKubeClient(t)
	c := NewClient(Config{})

	cfg := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "postgres"
		image = "postgres:16"
		config_db = true
		ports = [5432]
	`)
	require.NoError(t, c.ensureServices(context.Background(), k, cfg))

	name := serviceContainerName(cfg.Project, cfg.Services[0].ID)
	require.Contains(t, applied, "Deployment/"+name)
	require.Contains(t, applied, "Service/"+name,
		"Service should be applied for a config_db with declared ports")

	svc := appliedService(t, applied["Service/"+name])
	require.Len(t, svc.Spec.Ports, 1)
	require.Equal(t, projectDBPort, svc.Spec.Ports[0].Port)
}
