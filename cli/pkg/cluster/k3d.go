package cluster

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/clientcmd"

	k3dclient "github.com/k3d-io/k3d/v5/pkg/client"
	k3dcfg "github.com/k3d-io/k3d/v5/pkg/config"
	cfgtypes "github.com/k3d-io/k3d/v5/pkg/config/types"
	cfgv1alpha5 "github.com/k3d-io/k3d/v5/pkg/config/v1alpha5"
	k3dlogger "github.com/k3d-io/k3d/v5/pkg/logger"
	"github.com/k3d-io/k3d/v5/pkg/runtimes"
	k3dtypes "github.com/k3d-io/k3d/v5/pkg/types"
)

// k3dRuntime is the runtime for all k3d ops (Docker). Var so tests can swap.
var k3dRuntime = runtimes.SelectedRuntime

// loopbackHost keeps the published kube API and registry ports off the LAN.
// This is a single-machine dev cluster reached only by world-cli on the host.
const loopbackHost = "127.0.0.1"

// Host ↔ NodePort mappings for shared local services, published on the server
// node at cluster-create time so the API edge, operator, and project DB are
// reachable on localhost directly — no client-go port-forward to babysit. The node-side
// ports are NodePort-range (30000–32767) and MUST match the NodePort Services:
// traefikNodePort matches manifests/traefik/traefik.yaml; operatorNodePort
// matches the local NodePort Service applied in ensureOperator; dbNodePort
// matches projectDBService's NodePort. Because traffic is Service-routed, a pod
// restart simply re-targets a healthy pod.
const (
	apiHostPort      = 8080
	traefikNodePort  = 30080
	operatorHostPort = 8090
	operatorNodePort = 30090
	// dbHostPort exposes the auto-provisioned project DB on localhost so psql/GUIs
	// reach it without a port-forward. There's one shared DB per cluster, so a
	// single fixed mapping suffices. 5432 matches the docker backend's host port
	// (same psql habit).
	dbHostPort = 5432
	dbNodePort = 30432
)

// setK3dLogLevel routes a logrus level string into k3d's global logger AND
// installs the world-cli filter formatter (see k3dEssentialFormatter). Bad
// level strings fall back to Info rather than silently leaving the prior
// level. onLog nil routes to os.Stderr (k3d's original behavior); non-nil
// hands each line to the callback instead, so a caller with its own
// terminal renderer can display it without a second writer racing its
// redraws.
func setK3dLogLevel(level string, onLog func(line string)) {
	parsed, err := logrus.ParseLevel(level)
	if err != nil {
		parsed = logrus.InfoLevel
	}
	lg := k3dlogger.Log()
	lg.SetLevel(parsed)
	lg.SetFormatter(k3dEssentialFormatter{})
	lg.SetOutput(k3dLogWriter{onLog: onLog})
}

// k3dLogWriter adapts k3dEssentialFormatter's line-buffered output (each
// Write is one complete "...\n" entry) to an onLog callback.
type k3dLogWriter struct {
	onLog func(line string)
}

func (w k3dLogWriter) Write(p []byte) (int, error) {
	if w.onLog == nil {
		return os.Stderr.Write(p)
	}
	// k3dEssentialFormatter drops filtered entries by returning nil bytes;
	// os.Stderr.Write(nil) is a silent no-op, but a callback isn't — skip it
	// explicitly or every filtered line spams onLog("").
	if len(p) == 0 {
		return 0, nil
	}
	w.onLog(strings.TrimSuffix(string(p), "\n"))
	return len(p), nil
}

// k3dEssentialFormatter strips k3d's per-step INFO chatter (the "Created
// network", "Starting node", "Importing images from tarball" lines that add
// up to ~20 noise lines per cluster op) while keeping the signals world-cli
// users actually need:
//
//   - WARN / ERROR / FATAL / PANIC entries — pass through with their level
//     visible. Real problems should never be hidden.
//   - INFO entries whose message starts with "Pulling image" — long
//     operations (~30s for the k3s image on a cold machine) where silence
//     would be mistaken for a hang.
//
// Everything else returns nil bytes → logrus.Logger.Out.Write(nil) is a
// no-op, so the entry is dropped entirely.
type k3dEssentialFormatter struct{}

func (k3dEssentialFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if entry.Level <= logrus.WarnLevel {
		return fmt.Appendf(nil, "  [%s] %s\n", strings.ToUpper(entry.Level.String()), entry.Message), nil
	}
	if strings.HasPrefix(entry.Message, "Pulling image") {
		return []byte("  " + entry.Message + "\n"), nil
	}
	return nil, nil
}

// k3dExists reports whether a cluster with the given name is present. It is a
// package-level var (not a func) so tests can swap it without Docker; the
// default value points to the real implementation below.
//
//nolint:gochecknoglobals // Var so tests can swap.
var k3dExists = func(ctx context.Context, name string) (bool, error) {
	clusters, err := k3dclient.ClusterList(ctx, k3dRuntime)
	if err != nil {
		return false, eris.Wrap(err, "k3d cluster list")
	}
	for _, c := range clusters {
		if c.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// RequiredBootstrapImages returns the images a fresh cluster-create needs
// (k3s node, tools, load-balancer, registry), or nil if the cluster
// already exists (its node images are already present). Callers can
// pre-pull this list themselves before Start instead of pulling opaquely
// inside cluster-create.
func (c *Client) RequiredBootstrapImages(ctx context.Context) ([]string, error) {
	exists, err := k3dExists(ctx, c.cfg.ClusterName)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, nil
	}
	return []string{
		c.cfg.K3sImage,
		k3dtypes.GetToolsImage(),
		k3dtypes.GetLoadbalancerImage(),
		fmt.Sprintf("%s:%s", k3dtypes.DefaultRegistryImageRepo, k3dtypes.DefaultRegistryImageTag),
	}, nil
}

// k3dRunning reports whether the named cluster exists and has at least one
// running server node. A cluster paused with `k3d cluster stop` still exists
// (k3dExists returns true) but reports no running servers.
func k3dRunning(ctx context.Context, name string) (bool, error) {
	exists, err := k3dExists(ctx, name)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	cluster, err := k3dclient.ClusterGet(ctx, k3dRuntime, &k3dtypes.Cluster{Name: name})
	if err != nil {
		return false, eris.Wrap(err, "k3d cluster get")
	}
	_, running := cluster.ServerCountRunning()
	return running > 0, nil
}

// k3dCreate creates a cluster with attached registry. Mirrors
// `k3d cluster create <name> --registry-create <reg>` by walking the same
// process → transform → process → validate → run pipeline as the CLI; each
// step injects defaults later steps assume. It is a package-level var (not a
// func) so tests can swap it without Docker.
//
//nolint:gochecknoglobals // Var so tests can swap.
var k3dCreate = func(ctx context.Context, clusterName, registryName, k3sImage string) error {
	apiPort, err := getFreePort()
	if err != nil {
		return eris.Wrap(err, "find free port for kube API")
	}
	simple := cfgv1alpha5.SimpleConfig{
		TypeMeta:   cfgtypes.TypeMeta{APIVersion: "k3d.io/v1alpha5", Kind: "Simple"},
		ObjectMeta: cfgtypes.ObjectMeta{Name: clusterName},
		// Image: must be explicit; lib has no -ldflags default like the CLI.
		// ExposeAPI: must have an explicit HostPort; otherwise kubeconfig server
		// URL is portless and defaults to :443.
		Image:   k3sImage,
		Servers: 1,
		Agents:  0,
		ExposeAPI: cfgv1alpha5.SimpleExposureOpts{
			Host:     loopbackHost,
			HostIP:   loopbackHost,
			HostPort: apiPort,
		},
		// Publish the API edge + operator + project-DB NodePorts on loopback so the
		// host reaches them directly. server:0 targets the single server node,
		// where kube-proxy serves the NodePorts. The DB mapping is static (one
		// shared DB per cluster); host :5432 routes to a closed NodePort until a
		// world that needs a DB is deployed (connection-refused, same as the
		// operator endpoint during the no-world window).
		Ports: []cfgv1alpha5.PortWithNodeFilters{
			{
				Port:        fmt.Sprintf("%s:%d:%d/tcp", loopbackHost, apiHostPort, traefikNodePort),
				NodeFilters: []string{"server:0"},
			},
			{
				Port:        fmt.Sprintf("%s:%d:%d/tcp", loopbackHost, operatorHostPort, operatorNodePort),
				NodeFilters: []string{"server:0"},
			},
			{
				Port:        fmt.Sprintf("%s:%d:%d/tcp", loopbackHost, dbHostPort, dbNodePort),
				NodeFilters: []string{"server:0"},
			},
		},
		Registries: cfgv1alpha5.SimpleConfigRegistries{
			Create: &cfgv1alpha5.SimpleConfigRegistryCreateConfig{
				Name:     registryName,
				Host:     loopbackHost,
				HostPort: "random",
			},
		},
	}
	if err := k3dcfg.ProcessSimpleConfig(&simple); err != nil {
		return eris.Wrap(err, "process k3d simple config")
	}
	clusterCfg, err := k3dcfg.TransformSimpleToClusterConfig(ctx, k3dRuntime, simple, "")
	if err != nil {
		return eris.Wrap(err, "transform k3d cluster config")
	}
	clusterCfg, err = k3dcfg.ProcessClusterConfig(*clusterCfg)
	if err != nil {
		return eris.Wrap(err, "process k3d cluster config")
	}
	if err := k3dcfg.ValidateClusterConfig(ctx, k3dRuntime, *clusterCfg); err != nil {
		return eris.Wrap(err, "validate k3d cluster config")
	}
	if err := k3dclient.ClusterRun(ctx, k3dRuntime, clusterCfg); err != nil {
		return eris.Wrap(err, "k3d cluster run")
	}
	return nil
}

// k3dKubeconfig returns the cluster's kubeconfig as YAML bytes for
// clientcmd.RESTConfigFromKubeConfig. It is a package-level var (not a func)
// so tests can swap it without Docker.
//
//nolint:gochecknoglobals // Var so tests can swap.
var k3dKubeconfig = func(ctx context.Context, clusterName string) ([]byte, error) {
	cluster, err := k3dclient.ClusterGet(ctx, k3dRuntime, &k3dtypes.Cluster{Name: clusterName})
	if err != nil {
		return nil, eris.Wrap(err, "k3d cluster get")
	}
	cfg, err := k3dclient.KubeconfigGet(ctx, k3dRuntime, cluster)
	if err != nil {
		return nil, eris.Wrap(err, "k3d kubeconfig get")
	}
	out, err := clientcmd.Write(*cfg)
	if err != nil {
		return nil, eris.Wrap(err, "marshal kubeconfig")
	}
	return out, nil
}

// k3dDelete deletes the cluster (state lost). Used by Purge. It is a
// package-level var (not a func) so tests can swap it without Docker.
//
//nolint:gochecknoglobals // Var so tests can swap.
var k3dDelete = func(ctx context.Context, clusterName string) error {
	cluster, err := k3dclient.ClusterGet(ctx, k3dRuntime, &k3dtypes.Cluster{Name: clusterName})
	if err != nil {
		return eris.Wrap(err, "k3d cluster get")
	}
	if err := k3dclient.ClusterDelete(ctx, k3dRuntime, cluster, k3dtypes.ClusterDeleteOpts{}); err != nil {
		return eris.Wrap(err, "k3d cluster delete")
	}
	return nil
}

// k3dStop pauses cluster containers; state survives for a fast Start.
func k3dStop(ctx context.Context, clusterName string) error {
	cluster, err := k3dclient.ClusterGet(ctx, k3dRuntime, &k3dtypes.Cluster{Name: clusterName})
	if err != nil {
		return eris.Wrap(err, "k3d cluster get")
	}
	if err := k3dclient.ClusterStop(ctx, k3dRuntime, cluster); err != nil {
		return eris.Wrap(err, "k3d cluster stop")
	}
	return nil
}

// k3dStartIfStopped resumes a previously-stopped cluster. No-op if the
// cluster's server nodes are already running.
//
// Called from ensureCluster on the existing-but-stopped path: a fresh
// `world start` after `world stop` would otherwise return without ever
// restarting the k3s containers, and the next k8s API call would land on
// `https://127.0.0.1:<port>` with the host-side port mapping pointing at a
// stopped container — connection refused.
//
// Mirrors what the CLI's `k3d cluster start` does: gather environment info
// (the tools node populates HostGateway used by the DNS fix) and re-read
// stored start opts (HostAliases) from cluster node labels. Without
// EnvironmentInfo the per-node `enableFixes` step fails with "Cannot enable
// DNS fix, as Host Gateway IP is missing!". It is a package-level var (not a
// func) so tests can swap it without Docker.
//
//nolint:gochecknoglobals // Var so tests can swap.
var k3dStartIfStopped = func(ctx context.Context, clusterName string) error {
	cluster, err := k3dclient.ClusterGet(ctx, k3dRuntime, &k3dtypes.Cluster{Name: clusterName})
	if err != nil {
		return eris.Wrap(err, "k3d cluster get")
	}
	_, running := cluster.ServerCountRunning()
	if running > 0 {
		return nil
	}

	envInfo, err := k3dclient.GatherEnvironmentInfo(ctx, k3dRuntime, cluster)
	if err != nil {
		return eris.Wrap(err, "gather k3d environment info")
	}
	stored, err := k3dclient.GetClusterStartOptsFromLabels(cluster)
	if err != nil {
		return eris.Wrap(err, "read stored k3d start opts")
	}

	opts := k3dtypes.ClusterStartOpts{
		WaitForServer:   true,
		Intent:          k3dtypes.IntentClusterStart,
		EnvironmentInfo: envInfo,
		HostAliases:     stored.HostAliases,
	}
	if err := k3dclient.ClusterStart(ctx, k3dRuntime, cluster, opts); err != nil {
		return eris.Wrap(err, "k3d cluster start")
	}
	return nil
}

// k3dImageImport tars local images straight into each node's containerd,
// skipping the registry entirely (the `--registry-create` registry's random
// host port + non-resolving `k3d-<name>.localhost` hostname make `docker
// push` from the host brittle).
func k3dImageImport(ctx context.Context, clusterName string, images ...string) error {
	cluster, err := k3dclient.ClusterGet(ctx, k3dRuntime, &k3dtypes.Cluster{Name: clusterName})
	if err != nil {
		return eris.Wrap(err, "k3d cluster get")
	}
	// Mode pinned to ToolsNode (CLI default). AutoDetect's "direct" branch on
	// localhost Docker hits OrbStack volume-mount quirks that leave the tarball
	// unreachable from the server node.
	if err := k3dclient.ImageImportIntoClusterMulti(ctx, k3dRuntime, images, cluster, k3dtypes.ImageImportOpts{
		Mode: k3dtypes.ImportModeToolsNode,
	}); err != nil {
		return eris.Wrap(err, "k3d image import")
	}
	return nil
}

// getFreePort returns a free TCP port as a decimal string for k3d's API
// binding (empty leaves the kubeconfig server URL portless).
func getFreePort() (string, error) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		return "", err
	}
	defer func() { _ = l.Close() }()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port), nil
}
