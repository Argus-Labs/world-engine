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
	dynamicfake "k8s.io/client-go/dynamic/fake"

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
