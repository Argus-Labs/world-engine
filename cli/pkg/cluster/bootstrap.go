package cluster

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rotisserie/eris"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
	cardinaloperator "github.com/argus-labs/world-engine/cli/pkg/k8s/cardinal-operator"
)

// localOperatorImageEnv, when set, makes ensureOperator deploy a locally-built
// operator image instead of the pinned ECR image in the embedded manifest. The
// image is imported into the cluster and the manifest's container image is
// rewritten to it. Dev-only escape hatch for testing operator changes (e.g.
// `ko build --local --bare ./apps/cardinal-operator/cmd`) before a release.
const localOperatorImageEnv = "WORLD_LOCAL_OPERATOR_IMAGE"

// shardpoolCRDName is the metadata.name of the embedded ShardPool CRD,
// needed for the Established condition wait.
const shardpoolCRDName = "shardpools.cardinal.argus.gg"

// crdWaitTimeout bounds how long ensureCRD will wait for the CRD to report
// Established=True before erroring out.
const crdWaitTimeout = 60 * time.Second

// operatorNamespace is where the embedded operator manifest deploys
// cardinal-operator. The operator watches this same namespace (OPERATOR_NAMESPACE
// sourced from metadata.namespace via downward API), so ShardPool CRs land here
// too. Operator and shards co-locate here, mirroring prod's per-world overlays;
// one world at a time makes a shared namespace unambiguous.
const operatorNamespace = "cardinal-operator-system"

// ShardNamespace is the single namespace the editor deploys the operator and
// every world's shards into. Exported for the editor; see operatorNamespace.
func ShardNamespace() string { return operatorNamespace }

// Manifest paths inside cardinaloperator.Manifests (an embed.FS).
const (
	manifestCRD      = "manifests/crd/cardinal.argus.gg_shardpools.yaml"
	manifestOperator = "manifests/operator/operator.yaml"
	manifestNATS     = "manifests/nats/nats.yaml"
	manifestTraefik  = "manifests/traefik/traefik.yaml"
)

// ensureCluster makes sure a k3d cluster matching c.cfg is up and running.
// Three cases:
//   - doesn't exist → create from scratch with a built-in registry,
//   - exists + running → reuse as-is (start is idempotent),
//   - exists + stopped → restart (covers `world stop` followed by `world start`).
func (c *Client) ensureCluster(ctx context.Context) error {
	exists, err := k3dExists(ctx, c.cfg.ClusterName)
	if err != nil {
		return err
	}
	if !exists {
		return k3dCreate(ctx, c.cfg.ClusterName, c.cfg.RegistryName, c.cfg.K3sImage)
	}
	return k3dStartIfStopped(ctx, c.cfg.ClusterName)
}

// ensureCRD applies the ShardPool CRD and waits until the apiserver reports
// it Established. Required before any ShardPool CR can be applied.
func (c *Client) ensureCRD(ctx context.Context, k *kubeClient) error {
	doc, err := cardinaloperator.Manifests.ReadFile(manifestCRD)
	if err != nil {
		return eris.Wrap(err, "read embedded ShardPool CRD")
	}
	if err := k.applyYAML(ctx, doc); err != nil {
		return eris.Wrap(err, "apply ShardPool CRD")
	}
	waitCtx, cancel := context.WithTimeout(ctx, crdWaitTimeout)
	defer cancel()
	return k.waitForCRD(waitCtx, shardpoolCRDName)
}

// ensurePlatform applies the platform manifests into their dedicated namespaces.
// NATS and Traefik are cluster-shared across projects.
func (c *Client) ensurePlatform(ctx context.Context, k *kubeClient) error {
	natsDoc, err := cardinaloperator.Manifests.ReadFile(manifestNATS)
	if err != nil {
		return eris.Wrap(err, "read embedded NATS manifest")
	}
	if err := k.applyYAML(ctx, natsDoc); err != nil {
		return eris.Wrap(err, "apply NATS")
	}

	traefikDoc, err := cardinaloperator.Manifests.ReadFile(manifestTraefik)
	if err != nil {
		return eris.Wrap(err, "read embedded Traefik manifest")
	}
	if err := k.applyYAML(ctx, traefikDoc); err != nil {
		return eris.Wrap(err, "apply Traefik")
	}
	waitCtx, cancel := context.WithTimeout(ctx, crdWaitTimeout)
	defer cancel()
	if err := k.waitForCRD(waitCtx, "middlewares.traefik.io"); err != nil {
		return eris.Wrap(err, "wait for Traefik Middleware CRD")
	}
	return nil
}

// ensureOperator applies the cardinal-operator manifest. The embedded bundle
// pins the operator to a single namespace (operatorNamespace); projects share
// it.
func (c *Client) ensureOperator(ctx context.Context, k *kubeClient, shardDBDSN string) error {
	// The embedded operator manifest is namespace-scoped to operatorNamespace
	// but does not include its own Namespace doc (the kustomize overlay
	// intentionally drops it so the manifest can be applied into any
	// namespace by the caller). Create the namespace first.
	if err := c.applyNamespace(ctx, k, operatorNamespace); err != nil {
		return err
	}

	doc, err := cardinaloperator.Manifests.ReadFile(manifestOperator)
	if err != nil {
		return eris.Wrap(err, "read embedded cardinal-operator manifest")
	}
	// Dev escape hatch (see localOperatorImageEnv doc): import the local image
	// into the cluster and rewrite the manifest's container image to it.
	if localImage := strings.TrimSpace(os.Getenv(localOperatorImageEnv)); localImage != "" {
		if err := k3dImageImport(ctx, c.cfg.ClusterName, localImage); err != nil {
			return eris.Wrapf(err, "import local operator image %q", localImage)
		}
		doc = patchOperatorImage(doc, localImage)
	}
	// Point shards at the project DB (operator injects it as DB_DSN via SHARD_DB_DSN).
	if shardDBDSN != "" {
		doc = patchOperatorEnv(doc, "SHARD_DB_DSN", shardDBDSN)
	}

	if err := k.applyYAML(ctx, doc); err != nil {
		return err
	}

	// The embedded operator Service is ClusterIP (and is a CI-synced copy we do
	// not hand-edit), so add a local-only NodePort Service alongside it. Paired
	// with the k3d host-port mapping (127.0.0.1:8090 -> operatorNodePort), this
	// makes the operator reachable on localhost without a port-forward.
	nodePortSvc, err := operatorNodePortServiceYAML()
	if err != nil {
		return err
	}
	if err := k.applyYAML(ctx, nodePortSvc); err != nil {
		return eris.Wrap(err, "apply operator NodePort service")
	}
	return nil
}

// operatorNodePortServiceYAML renders the local-only NodePort Service that fronts
// the operator. Built from the typed corev1.Service so the field shapes (port
// kinds, NodePort range, selector) are compile-checked rather than hand-written
// YAML.
func operatorNodePortServiceYAML() ([]byte, error) {
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cardinal-operator-nodeport",
			Namespace: operatorNamespace,
			Labels:    map[string]string{"managed-by": "world-cli"},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: map[string]string{"app.kubernetes.io/name": "cardinal-operator"},
			Ports: []corev1.ServicePort{{
				Name:       "api",
				Protocol:   corev1.ProtocolTCP,
				Port:       operatorHostPort,
				TargetPort: intstr.FromInt32(operatorHostPort),
				NodePort:   operatorNodePort,
			}},
		},
	}
	doc, err := yaml.Marshal(svc)
	if err != nil {
		return nil, eris.Wrap(err, "marshal operator NodePort service")
	}
	return doc, nil
}

// patchOperatorImage rewrites the operator container's image in the embedded
// manifest to localImage and forces IfNotPresent so the imported node-local
// image is used without a registry lookup (the operator.yaml has a single
// container `image:` line). Matches by the `image:` key, not the old value, so
// it survives manifest tag bumps.
func patchOperatorImage(doc []byte, localImage string) []byte {
	lines := strings.Split(string(doc), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "image:") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		lines[i] = indent + "image: " + localImage + "\n" + indent + "imagePullPolicy: IfNotPresent"
		break
	}
	return []byte(strings.Join(lines, "\n"))
}

// patchOperatorEnv inserts a name/value pair into the CI-synced manifest's env block at apply time.
func patchOperatorEnv(doc []byte, name, value string) []byte {
	lines := strings.Split(string(doc), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "env:" {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		lines[i] = line + "\n" + indent + "- name: " + name + "\n" + indent + "  value: " + strconv.Quote(value)
		break
	}
	return []byte(strings.Join(lines, "\n"))
}

// applyNamespace ensures a namespace exists via server-side apply (idempotent).
func (c *Client) applyNamespace(ctx context.Context, k *kubeClient, name string) error {
	doc := []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: " + name + "\n")
	if err := k.applyYAML(ctx, doc); err != nil {
		return eris.Wrapf(err, "ensure namespace %s", name)
	}
	return nil
}

// applyShardPools converts each cluster.ShardPool into the ShardPool CR shape
// and applies it into the operator's watch namespace.
func (c *Client) applyShardPools(ctx context.Context, k *kubeClient, pools []ShardPool) error {
	for _, p := range pools {
		doc, err := shardPoolYAML(p)
		if err != nil {
			return eris.Wrapf(err, "render ShardPool/%s", p.ShardID)
		}
		if err := k.applyYAML(ctx, doc); err != nil {
			return eris.Wrapf(err, "apply ShardPool/%s", p.ShardID)
		}
	}
	return nil
}

// LocalShardAPIURL returns the Traefik-routed Cardinal API URL for a shard
// instance on the local cluster.
func LocalShardAPIURL(organization, project, instanceName string) string {
	return fmt.Sprintf("http://localhost:%d/%s/%s/%s",
		apiHostPort,
		dnslabel.Sanitize(organization),
		dnslabel.Sanitize(project),
		dnslabel.Sanitize(instanceName),
	)
}

// shardPoolCR and friends are the wire shape of a ShardPool custom resource.
// Field names come from the json tags (sigs.k8s.io/yaml is json-tag based).
// omitempty on the optional spec fields reproduces the previous template's
// conditional emission; poolSize and the resource values are always emitted.
type shardPoolCR struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   shardPoolMetadata `json:"metadata"`
	Spec       shardPoolSpec     `json:"spec"`
}

type shardPoolMetadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type shardPoolSpec struct {
	ShardID      string             `json:"shardID"`
	Organization string             `json:"organization"`
	Project      string             `json:"project"`
	Region       string             `json:"region"`
	Image        string             `json:"image"`
	TickRate     int32              `json:"tickRate,omitempty"`
	Mode         string             `json:"mode,omitempty"`
	PoolSize     int32              `json:"poolSize"`
	LogLevel     string             `json:"logLevel,omitempty"`
	Resources    shardPoolResources `json:"resources"`
}

type shardPoolResources struct {
	Requests shardPoolResourceValues `json:"requests"`
	Limits   shardPoolResourceValues `json:"limits"`
}

type shardPoolResourceValues struct {
	CPU    int32 `json:"cpu"`
	Memory int32 `json:"memory"`
}

// shardPoolYAML marshals a ShardPool CR targeted at the operator's watch
// namespace. It builds a typed value and lets a real YAML encoder handle
// quoting/escaping, so arbitrary config-sourced strings (mode, image, org, ...)
// cannot break out of their field — a hand-quoted template emits malformed YAML
// for a value like `LEADER"hello`. A local CR shape (rather than the operator's
// typed API) keeps this free of a heavy dep cycle.
//
// imageTag is intentionally omitted from the spec — operator-owned at runtime.
// The shape mirrors
// infra/k8s/apps/cardinal-operator/overlays/<env>/shardpools.yaml.
func shardPoolYAML(p ShardPool) ([]byte, error) {
	cr := shardPoolCR{
		APIVersion: shardPoolGroup + "/" + shardPoolVersion,
		Kind:       shardPoolKind,
		Metadata: shardPoolMetadata{
			Name:      p.ShardID,
			Namespace: operatorNamespace,
		},
		Spec: shardPoolSpec{
			ShardID:      p.ShardID,
			Organization: p.Organization,
			Project:      p.Project,
			Region:       p.Region,
			Image:        p.Image,
			TickRate:     p.TickRate,
			Mode:         p.Mode,
			PoolSize:     p.PoolSize,
			LogLevel:     p.LogLevel,
			Resources: shardPoolResources{
				Requests: shardPoolResourceValues{
					CPU:    p.Resources.Requests.CPU,
					Memory: p.Resources.Requests.Memory,
				},
				Limits: shardPoolResourceValues{
					CPU:    p.Resources.Limits.CPU,
					Memory: p.Resources.Limits.Memory,
				},
			},
		},
	}
	doc, err := yaml.Marshal(cr)
	if err != nil {
		return nil, eris.Wrap(err, "marshal ShardPool CR")
	}
	return doc, nil
}
