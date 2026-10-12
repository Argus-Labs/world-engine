package cluster

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/rotisserie/eris"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// Labels marking a [[services]] Deployment+Service as world-cli-owned, scoped per
// project so Start can GC the entries dropped from world.toml (multi-project safe).
const (
	managedByLabel = "world-cli-managed"
	projectLabel   = "world-cli-project"
)

// managedServiceLabels are the object labels on a [[services]] Deployment+Service:
// "app" for pod selection plus the world-cli-managed/project markers GC keys off.
func managedServiceLabels(name, project string) map[string]string {
	return map[string]string{
		"app":          name,
		managedByLabel: "true",
		projectLabel:   project,
	}
}

// serviceContainerName returns the k8s Deployment/Service name for a [[services]]
// entry ("{project}-{id}-service"). Keyed off the project (mirroring docker's
// GameServiceContainerName) so DSN hosts stay deterministic.
func serviceContainerName(project, id string) string {
	return fmt.Sprintf("%s-%s-service", project, id)
}

// serviceEnv builds a [[services]] entry's env. It seeds the Cardinal wire
// identity (CARDINAL_REGION/ORG/PROJECT/SHARD_ID — the same set the operator
// gives shards, so a service like meta subscribes on the right NATS subject) plus
// NATS_URL; svc.Env (world.toml) overrides them. DB_DSN (db = true) is filled only
// when absent; POSTGRES_* (config_db) always win. Sorted by name for determinism.
func serviceEnv(cfg toml.Config, svc toml.GameService) []corev1.EnvVar {
	merged := map[string]string{
		"NATS_URL":          natsClusterURL,
		"CARDINAL_REGION":   DefaultRegion,
		"CARDINAL_ORG":      cfg.Organization,
		"CARDINAL_PROJECT":  cfg.Project,
		"CARDINAL_SHARD_ID": svc.ID,
	}
	maps.Copy(merged, svc.Env)
	if svc.DB {
		if dsn := projectDBDSN(cfg); dsn != "" {
			if _, ok := merged[dbDSNEnvVar]; !ok {
				merged[dbDSNEnvVar] = dsn
			}
		}
	}
	if svc.ConfigDB {
		merged["POSTGRES_USER"] = configDBUser
		merged["POSTGRES_PASSWORD"] = configDBPassword
		merged["POSTGRES_DB"] = cfg.Project
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	env := make([]corev1.EnvVar, 0, len(merged))
	for _, k := range keys {
		env = append(env, corev1.EnvVar{Name: k, Value: merged[k]})
	}
	return env
}

// serviceImage resolves a [[services]] entry's image: svc.Image for image-kind,
// or the local-registry ref for path-kind. Path-kind images must be built +
// imported by the world-cli start flow (wired separately) or the Deployment
// ImagePullBackOffs.
func serviceImage(c *Client, cfg toml.Config, svc toml.GameService) string {
	if svc.IsBuiltFromSource() {
		return c.imageRef(cfg.Project, svc.ID)
	}
	return svc.Image
}

// ensureServices applies a Deployment (+ Service when ports are declared) per
// [[services]] entry into operatorNamespace. Readiness is intentionally not
// awaited here — path-kind images aren't built in this increment, so a hard wait
// would always time out; the start flow builds/imports them separately.
func (c *Client) ensureServices(ctx context.Context, k *kubeClient, cfg toml.Config) error {
	ns := projectServiceNamespace()
	// Apply config_db services first so a db=true service declared before its
	// config_db in world.toml doesn't briefly crash-loop against a DB whose
	// Deployment hasn't landed yet. (No readiness wait here — k8s reconciles it.)
	svcs := slices.Clone(cfg.Services)
	slices.SortStableFunc(svcs, func(a, b toml.GameService) int {
		switch {
		case a.ConfigDB == b.ConfigDB:
			return 0
		case a.ConfigDB:
			return -1
		default:
			return 1
		}
	})
	for _, svc := range svcs {
		// Path-kind services are built+imported+deployed by DeployServices in the
		// start flow; applying them here would ImagePullBackOff on a missing image.
		if svc.IsBuiltFromSource() {
			continue
		}
		name := serviceContainerName(cfg.Project, svc.ID)

		dep := serviceDeployment(name, ns, cfg.Project, serviceImage(c, cfg, svc), serviceEnv(cfg, svc), svc.Ports)
		doc, err := yaml.Marshal(dep)
		if err != nil {
			return eris.Wrapf(err, "marshal service %s", name)
		}
		if err := k.applyYAML(ctx, doc); err != nil {
			return eris.Wrapf(err, "apply service %s", name)
		}

		if len(svc.Ports) > 0 {
			svcDoc, err := yaml.Marshal(serviceService(name, ns, cfg.Project, svc.Ports))
			if err != nil {
				return eris.Wrapf(err, "marshal service %s Service", name)
			}
			if err := k.applyYAML(ctx, svcDoc); err != nil {
				return eris.Wrapf(err, "apply service %s Service", name)
			}
		}
	}
	return nil
}

// gcOrphanedServices deletes managed [[services]] Deployments and Services in the
// project namespace whose names are no longer declared in cfg.Services. Without
// it, services dropped from world.toml linger across re-runs — the operator GCs
// shards via its own labels, but world-cli's services aren't covered. Scoped per
// project via projectLabel so it stays multi-project safe. A list failure is
// surfaced (a real cluster problem); non-NotFound delete failures are logged to
// stderr but never block Start (best-effort cleanup).
func gcOrphanedServices(ctx context.Context, k *kubeClient, cfg toml.Config) error {
	ns := projectServiceNamespace()
	expected := make(map[string]struct{}, len(cfg.Services))
	for _, svc := range cfg.Services {
		expected[serviceContainerName(cfg.Project, svc.ID)] = struct{}{}
	}

	deployGVR := appsv1.SchemeGroupVersion.WithResource("deployments")
	svcGVR := corev1.SchemeGroupVersion.WithResource("services")
	selector := fmt.Sprintf("%s=true,%s=%s", managedByLabel, projectLabel, cfg.Project)

	// Reap Deployments and Services independently: a Service can outlive its
	// Deployment (e.g. a prior partial delete), so listing only Deployments would
	// leak it. The auto project DB ({project}-db) is intentionally NOT reaped — it
	// carries no managedByLabel, so its data survives world.toml edits.
	for _, t := range []struct {
		ri   dynamic.ResourceInterface
		kind string
	}{
		{k.dynamic.Resource(deployGVR).Namespace(ns), "Deployment"},
		{k.dynamic.Resource(svcGVR).Namespace(ns), "Service"},
	} {
		list, err := t.ri.List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return eris.Wrapf(err, "list managed %ss", t.kind)
		}
		for i := range list.Items {
			name := list.Items[i].GetName()
			if _, ok := expected[name]; ok {
				continue
			}
			if err := t.ri.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				fmt.Fprintf(os.Stderr, "world-cli: delete orphaned %s %s/%s: %v\n", t.kind, ns, name, err)
			}
		}
	}
	return nil
}

func serviceDeployment(name, ns, project, image string, env []corev1.EnvVar, ports []int) *appsv1.Deployment {
	podLabels := map[string]string{"app": name}
	containerPorts := make([]corev1.ContainerPort, 0, len(ports))
	for _, p := range ports {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			ContainerPort: int32(p),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	return &appsv1.Deployment{
		APIVersion: "apps/v1", Kind: "Deployment",
		Name: name, Namespace: ns, Labels: managedServiceLabels(name, project),
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: podLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  name,
						Image: image,
						// IfNotPresent so a locally-imported (path-kind) or already-pulled
						// image-kind tag isn't re-pulled from a registry — k8s otherwise
						// defaults to Always for :latest, which fails in offline k3d.
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env:             env,
						Ports:           containerPorts,
					}},
				},
			},
		},
	}
}

func serviceService(name, ns, project string, ports []int) *corev1.Service {
	servicePorts := make([]corev1.ServicePort, 0, len(ports))
	for _, p := range ports {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", p),
			Port:       int32(p),
			TargetPort: intstr.FromInt(p),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	return &corev1.Service{
		APIVersion: "v1", Kind: "Service",
		Name: name, Namespace: ns, Labels: managedServiceLabels(name, project),
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": name},
			Ports:    servicePorts,
		},
	}
}
