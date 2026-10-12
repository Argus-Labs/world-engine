package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/rotisserie/eris"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/yaml"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// These mirror the docker backend defaults in pkg/docker/service so both
// backends provision the shared project DB with identical image/creds/port.
const (
	projectDBImage         = "postgres:16"
	configDBUser           = "postgres"
	configDBPassword       = "postgres"
	projectDBPort    int32 = 5432
	dbDSNEnvVar            = "DB_DSN"

	// natsClusterURL is the in-cluster NATS address, derived from the Service
	// name/namespace/port in pkg/k8s/cardinal-operator/manifests/nats/nats.yaml.
	natsClusterURL = "nats://nats.nats.svc.cluster.local:4222"
)

// dbReadyTimeout bounds the wait for the auto-provisioned DB Deployment to go
// Available before dependents are applied.
const dbReadyTimeout = 120 * time.Second

// projectServiceNamespace returns the namespace project DBs + services land in.
// Co-located with shards for now; per-project namespace is deferred.
func projectServiceNamespace() string { return operatorNamespace }

// projectDBName returns the auto-provisioned DB's name ("{project}-db"), used as
// the in-cluster Service host.
func projectDBName(project string) string { return project + "-db" }

// anyServiceUsesDB reports whether any [[services]] entry sets db = true.
func anyServiceUsesDB(cfg toml.Config) bool {
	for _, svc := range cfg.Services {
		if svc.DB {
			return true
		}
	}
	return false
}

// hasConfigDBService reports whether world.toml declares a config_db service.
func hasConfigDBService(cfg toml.Config) bool {
	for _, svc := range cfg.Services {
		if svc.ConfigDB {
			return true
		}
	}
	return false
}

// projectDBDSN returns the shared DB_DSN — one database per project. It prefers a
// declared config_db service (host = its k8s Service name), else the
// auto-provisioned "{project}-db". The host is always an in-cluster FQDN.
// Returns "" when the project uses no database.
func projectDBDSN(cfg toml.Config) string {
	ns := projectServiceNamespace()
	var host string
	switch {
	case hasConfigDBService(cfg):
		for _, svc := range cfg.Services {
			if svc.ConfigDB {
				host = serviceContainerName(cfg.Project, svc.ID)
				break
			}
		}
	case anyServiceUsesDB(cfg):
		host = projectDBName(cfg.Project)
	default:
		return ""
	}
	return clusterDSN(host, ns, cfg.Project)
}

// clusterDSN builds the in-cluster Postgres DSN for a Service host (the single Sprintf site).
func clusterDSN(host, ns, project string) string {
	fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", host, ns)
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		configDBUser, configDBPassword, fqdn, projectDBPort, project,
	)
}

// shardDBDSN returns the pod DB_DSN: a config_db service's DB when declared, else "{project}-db"; never "".
func shardDBDSN(cfg toml.Config) string {
	if dsn := projectDBDSN(cfg); dsn != "" {
		return dsn
	}
	return clusterDSN(projectDBName(cfg.Project), projectServiceNamespace(), cfg.Project)
}

// ensureProjectDB provisions the auto shared Postgres (PVC + Deployment +
// Service) and waits for it to go Available. It always provisions when called;
// the caller in client.go (DeployWorld) gates the call on !hasConfigDBService —
// a config_db service supplies its own DB, so nothing is provisioned here.
func ensureProjectDB(ctx context.Context, k *kubeClient, project string) error {
	ns := projectServiceNamespace()
	name := projectDBName(project)

	for _, obj := range []any{
		projectDBPVC(name, ns),
		projectDBDeployment(name, ns, project),
		projectDBService(name, ns),
	} {
		doc, err := yaml.Marshal(obj)
		if err != nil {
			return eris.Wrapf(err, "marshal project DB %s", name)
		}
		if err := k.applyYAML(ctx, doc); err != nil {
			return eris.Wrapf(err, "apply project DB %s", name)
		}
	}

	waitCtx, cancel := context.WithTimeout(ctx, dbReadyTimeout)
	defer cancel()
	return waitForDeploymentReady(waitCtx, k, ns, name)
}

func projectDBPVC(name, ns string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		APIVersion: "v1", Kind: "PersistentVolumeClaim",
		Name: name, Namespace: ns,
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
}

func projectDBDeployment(name, ns, project string) *appsv1.Deployment {
	podLabels := map[string]string{"app": name}
	return &appsv1.Deployment{
		APIVersion: "apps/v1", Kind: "Deployment",
		Name: name, Namespace: ns, Labels: podLabels,
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(1)),
			// Recreate so the new pod can attach the ReadWriteOnce PVC without
			// contending with the outgoing one.
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: podLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  name,
						Image: projectDBImage,
						Env: []corev1.EnvVar{
							{Name: "POSTGRES_USER", Value: configDBUser},
							{Name: "POSTGRES_PASSWORD", Value: configDBPassword},
							{Name: "POSTGRES_DB", Value: project},
						},
						Ports: []corev1.ContainerPort{{ContainerPort: projectDBPort, Protocol: corev1.ProtocolTCP}},
						// Modest requests/limits so k3d's scheduler can size the pod; without
						// them Postgres can take unbounded memory and OOM under CI load.
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "data", MountPath: "/var/lib/postgresql/data"},
						},
						ReadinessProbe: &corev1.Probe{
							Exec: &corev1.ExecAction{
								Command: []string{"pg_isready", "-U", configDBUser, "-d", project},
							},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:                  "data",
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name},
					}},
				},
			},
		},
	}
}

// projectDBService fronts the project DB. It's a NodePort (not ClusterIP) so the
// host reaches it on localhost:5432 via the k3d port mapping (dbHostPort →
// dbNodePort) with no port-forward; in-cluster consumers still use the Service
// DNS name as before (NodePort is a ClusterIP superset).
func projectDBService(name, ns string) *corev1.Service {
	return &corev1.Service{
		APIVersion: "v1", Kind: "Service",
		Name: name, Namespace: ns, Labels: map[string]string{"app": name},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{{
				Name:       "postgres",
				Port:       projectDBPort,
				TargetPort: intstr.FromInt32(projectDBPort),
				NodePort:   dbNodePort,
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

// waitForDeploymentReady blocks until the named Deployment reports Available=True
// (or availableReplicas >= 1) or ctx expires. Polls every 500ms, mirroring
// waitForCRD.
func waitForDeploymentReady(ctx context.Context, k *kubeClient, namespace, name string) error {
	gvr := appsv1.SchemeGroupVersion.WithResource("deployments")
	var lastErr error
	err := wait.PollUntilContextCancel(ctx, 500*time.Millisecond, true,
		func(ctx context.Context) (bool, error) {
			obj, err := k.dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				// Ignore our own deadline cancelling the Get; keep real API
				// failures (RBAC, unreachable) for the timeout message.
				if ctx.Err() == nil {
					lastErr = err
				}
				return false, nil
			}
			return deploymentAvailable(obj), nil
		})
	if err == nil {
		return nil
	}
	// Surface the last poll error (e.g. RBAC/API failure) instead of a bare
	// deadline, so a persistent Get failure isn't masked as a plain timeout.
	if lastErr != nil {
		return eris.Wrapf(lastErr, "wait for Deployment %s/%s available: polling kept failing", namespace, name)
	}
	return eris.Wrapf(ctx.Err(), "wait for Deployment %s/%s available", namespace, name)
}

// deploymentAvailable reports whether a Deployment's status has an Available=True
// condition or at least one available replica.
func deploymentAvailable(obj *unstructured.Unstructured) bool {
	if n, found, err := unstructured.NestedInt64(
		obj.Object,
		"status",
		"availableReplicas",
	); found && err == nil &&
		n >= 1 {
		return true
	}
	conds, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found || err != nil {
		return false
	}
	for _, raw := range conds {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if c["type"] == "Available" && c["status"] == "True" {
			return true
		}
	}
	return false
}
