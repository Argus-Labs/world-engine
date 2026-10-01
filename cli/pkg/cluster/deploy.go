package cluster

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/moby/moby/client"
	"github.com/rotisserie/eris"
	"sigs.k8s.io/yaml"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
	operatorv1 "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1"
)

// DeployShard pairs a shard ID with the already-built local image tag.
type DeployShard struct {
	ID          string // matches ShardPool.shardID
	SourceImage string // e.g. "<project>-<id>-shard:latest" from pkg/docker
}

// Deploy retags each shard's built image to the registry-prefixed ref the
// operator expects, imports it into the cluster's containerd, and calls the
// operator's Deploy RPC. Caller builds the images first (via pkg/docker).
//
// Import instead of push: `k3d-<name>.localhost` doesn't resolve on host and
// the registry's host port is randomized. Importing tars the image straight
// into containerd; IfNotPresent pull-policy then skips the registry lookup.
func (c *Client) Deploy(ctx context.Context, opts DeployOpts) error {
	if opts.Project == "" {
		return eris.New("Deploy: project is required")
	}
	if len(opts.Shards) == 0 {
		return eris.New("Deploy: at least one shard is required")
	}

	docker, err := client.New(client.FromEnv)
	if err != nil {
		return eris.Wrap(err, "docker client")
	}
	defer func() { _ = docker.Close() }()

	tag := uniqueTag()
	rpc := c.operatorClient()

	step := func(shardID, label string) {
		if opts.OnStep != nil {
			opts.OnStep(shardID, label)
		}
	}

	for _, s := range opts.Shards {
		dest := fmt.Sprintf("%s:%s", c.imageRef(opts.Project, s.ID), tag)

		step(s.ID, "tagging image")
		if _, err := docker.ImageTag(ctx, client.ImageTagOptions{Source: s.SourceImage, Target: dest}); err != nil {
			return eris.Wrapf(err, "tag %s for shard %s", s.SourceImage, s.ID)
		}
		step(s.ID, "importing into cluster")
		if err := k3dImageImport(ctx, c.cfg.ClusterName, dest); err != nil {
			return eris.Wrapf(err, "import %s for shard %s", dest, s.ID)
		}

		step(s.ID, "rolling pods")
		req := connect.NewRequest(&operatorv1.DeployRequest{
			ShardId:  s.ID,
			ImageTag: tag,
		})
		if _, err := rpc.Deploy(ctx, req); err != nil {
			return eris.Wrapf(err, "operator Deploy RPC for shard %s", s.ID)
		}
	}
	return nil
}

// uniqueTag guarantees a fresh image string each call. The operator rolls
// pods only on image-string change (IfNotPresent pull-policy).
func uniqueTag() string {
	return fmt.Sprintf("local-%d", time.Now().UnixNano())
}

// DeployedWorldKeys returns the WorldKey ("{org}/{project}") of every world with
// ShardPools on the cluster, letting the editor re-derive which saved worlds are
// live after a restart. At most one entry (one world at a time).
//
// Reads the ShardPool CRs straight from the apiserver, not via the operator's
// Status RPC: the operator only runs while a world is deployed, so querying it
// would fail in exactly the "is anything running?" case this answers.
func (c *Client) DeployedWorldKeys(ctx context.Context) (map[string]struct{}, error) {
	k, err := c.kube(ctx)
	if err != nil {
		return nil, err
	}
	return k.listShardPoolWorldKeys(ctx, operatorNamespace)
}

// DeployServicesOpts controls DeployServices behavior.
type DeployServicesOpts struct {
	Project string
	Config  toml.Config
}

// serviceSourceImage is the local image buildImagesWithProgress produces for a
// path-kind [[services]] entry (GameServiceFromConfig sets Image == Name).
func serviceSourceImage(project, id string) string {
	return serviceContainerName(project, id) + ":latest"
}

// DeployServices is the path-kind counterpart to Deploy: it retags each
// source-built [[services]] image to the registry-prefixed ref, imports it into
// the cluster's containerd, and re-applies the Deployment/Service with the
// imported tag. There's no operator RPC for services — k8s rolls the pod itself
// when the container image string changes. Caller builds the images first (via
// pkg/docker). A clean no-op when no path-kind services are declared.
func (c *Client) DeployServices(ctx context.Context, opts DeployServicesOpts) error {
	if opts.Project == "" {
		return eris.New("DeployServices: project is required")
	}

	docker, err := client.New(client.FromEnv)
	if err != nil {
		return eris.Wrap(err, "docker client")
	}
	defer func() { _ = docker.Close() }()

	kubeconfig, err := k3dKubeconfig(ctx, c.cfg.ClusterName)
	if err != nil {
		return err
	}
	k, err := newKubeClient(kubeconfig)
	if err != nil {
		return err
	}

	tag := uniqueTag()
	ns := projectServiceNamespace()

	for _, svc := range opts.Config.Services {
		if !svc.IsBuiltFromSource() {
			continue
		}
		name := serviceContainerName(opts.Project, svc.ID)
		source := serviceSourceImage(opts.Project, svc.ID)
		dest := fmt.Sprintf("%s:%s", c.imageRef(opts.Project, svc.ID), tag)

		if _, err := docker.ImageTag(ctx, client.ImageTagOptions{Source: source, Target: dest}); err != nil {
			return eris.Wrapf(err, "tag %s for service %s", source, svc.ID)
		}
		if err := k3dImageImport(ctx, c.cfg.ClusterName, dest); err != nil {
			return eris.Wrapf(err, "import %s for service %s", dest, svc.ID)
		}

		// Re-apply with the imported tag so k8s rolls the pod onto the new image.
		dep, err := yaml.Marshal(
			serviceDeployment(name, ns, opts.Project, dest, serviceEnv(opts.Config, svc), svc.Ports),
		)
		if err != nil {
			return eris.Wrapf(err, "marshal service %s", svc.ID)
		}
		if err := k.applyYAML(ctx, dep); err != nil {
			return eris.Wrapf(err, "apply service %s", svc.ID)
		}
		if len(svc.Ports) > 0 {
			svcDoc, err := yaml.Marshal(serviceService(name, ns, opts.Project, svc.Ports))
			if err != nil {
				return eris.Wrapf(err, "marshal service %s Service", svc.ID)
			}
			if err := k.applyYAML(ctx, svcDoc); err != nil {
				return eris.Wrapf(err, "apply service %s Service", svc.ID)
			}
		}
	}
	return nil
}

// DeployedShard describes a shard deployed on the cluster and the world
// (organization/project) it belongs to.
type DeployedShard struct {
	ShardID      string
	Organization string
	Project      string
}

// DeployedShards lists the shards currently deployed on the cluster (from
// ShardPool CRs) with their owning world's organization/project. This lets
// callers address a shard via the Gateway without a local world.toml.
func (c *Client) DeployedShards(ctx context.Context) ([]DeployedShard, error) {
	var out []DeployedShard
	err := c.withKube(ctx, func(k *kubeClient) error {
		shards, listErr := k.listDeployedShards(ctx, operatorNamespace)
		if listErr != nil {
			return listErr
		}
		out = shards
		return nil
	})
	return out, err
}
