// Package cluster reads shards on a Kubernetes cluster (hosted dev server,
// ephemeral environments) through the developer's kubeconfig: status and logs
// only. Local worlds run on Docker (cli/pkg/local); nothing here deploys.
package cluster

import (
	"context"
	"sync"

	"github.com/rotisserie/eris"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Labels the cardinal-shard chart puts on every shard pod.
const (
	shardIDLabel  = "cardinal.argus.gg/shard-id"
	instanceLabel = "cardinal.argus.gg/instance"
)

// Config selects the kubeconfig and context; empty means the kubeconfig's current context.
type Config struct {
	Kubeconfig string
	Context    string
}

// Client is a lazily connected, reusable Kubernetes reader.
type Client struct {
	cfg Config

	mu          sync.Mutex
	clientset   kubernetes.Interface
	contextName string
}

func NewClient(cfg Config) *Client { return &Client{cfg: cfg} }

// ContextName returns the kubeconfig context in use.
func (c *Client) ContextName(ctx context.Context) (string, error) {
	if _, err := c.kube(ctx); err != nil {
		return "", err
	}
	return c.contextName, nil
}

func (c *Client) kube(_ context.Context) (kubernetes.Interface, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.clientset != nil {
		return c.clientset, nil
	}
	rc, name, err := loadKubeconfig(c.cfg)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, eris.Wrapf(err, "kubernetes client for context %q", name)
	}
	c.clientset, c.contextName = cs, name
	return cs, nil
}

func loadKubeconfig(cfg Config) (*rest.Config, string, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		rules.ExplicitPath = cfg.Kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: cfg.Context}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	raw, err := cc.RawConfig()
	if err != nil {
		return nil, "", eris.Wrap(err, "read kubeconfig")
	}
	name := cfg.Context
	if name == "" {
		name = raw.CurrentContext
	}
	if name == "" {
		return nil, "", eris.New("kubeconfig has no current context; pass --context or set KUBECONFIG")
	}
	rc, err := cc.ClientConfig()
	if err != nil {
		return nil, "", eris.Wrapf(err, "kubeconfig context %q", name)
	}
	return rc, name, nil
}

// ProjectNamespace is the namespace a project's shards live in by convention.
func ProjectNamespace(project string) string { return project }
