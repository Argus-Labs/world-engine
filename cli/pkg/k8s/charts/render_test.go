package charts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"sigs.k8s.io/yaml"

	"github.com/argus-labs/world-engine/cli/pkg/version"
)

// renderFiles does what `helm template` does: load, resolve dependencies, merge values,
// validate against values.schema.json (ToRenderValues does this), render.
func renderFiles(chartDir string, vals map[string]any) (map[string]string, error) {
	chrt, err := loader.Load(chartDir)
	if err != nil {
		return nil, err
	}
	if err := chartutil.ProcessDependencies(chrt, vals); err != nil {
		return nil, err
	}
	merged, err := chartutil.ToRenderValues(chrt, vals,
		chartutil.ReleaseOptions{Name: "test", Namespace: "test"}, chartutil.DefaultCapabilities)
	if err != nil {
		return nil, err
	}
	return engine.Render(chrt, merged)
}

func loadValues(t *testing.T, valuesFile string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatal(err)
	}
	var vals map[string]any
	if err := yaml.Unmarshal(raw, &vals); err != nil {
		t.Fatalf("%s: %v", valuesFile, err)
	}
	return vals
}

// render templates the chart with one values file and returns object names by kind.
func render(t *testing.T, chartDir, valuesFile string) map[string][]string {
	t.Helper()
	return renderVals(t, chartDir, loadValues(t, valuesFile))
}

func renderVals(t *testing.T, chartDir string, vals map[string]any) map[string][]string {
	t.Helper()
	files, err := renderFiles(chartDir, vals)
	if err != nil {
		t.Fatalf("render %s: %v", chartDir, err)
	}
	out := map[string][]string{}
	for name, body := range files {
		if filepath.Ext(name) != ".yaml" {
			continue
		}
		for doc := range strings.SplitSeq(body, "\n---") {
			var obj struct {
				Kind     string `json:"kind"`
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
			}
			if err := yaml.Unmarshal([]byte(doc), &obj); err != nil || obj.Kind == "" {
				continue
			}
			out[obj.Kind] = append(out[obj.Kind], obj.Metadata.Name)
		}
	}
	return out
}

// renderErr templates the chart expecting a schema or template error containing want.
func renderErr(t *testing.T, chartDir string, vals map[string]any, want string) {
	t.Helper()
	_, err := renderFiles(chartDir, vals)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want error containing %q, got %v", want, err)
	}
}

// minimal is a values set that passes the final schema; tests override one key to break it.
func minimal() map[string]any {
	return map[string]any{
		"shardID": "a", "image": "x", "imageTag": "y", "poolSize": 1,
		"organization": "o", "project": "p", "region": "r",
		"auth": map[string]any{"mode": "DEV"},
		"nats": map[string]any{"url": "nats://n:4222"},
	}
}

const shardChart = "cardinal-shard"

func TestShardChartNoRoutingByDefault(t *testing.T) {
	got := render(t, shardChart, "cardinal-shard/examples/local.yaml")
	for _, kind := range []string{"HTTPRoute", "HealthCheckPolicy", "GCPBackendPolicy"} {
		if len(got[kind]) != 0 {
			t.Fatalf("%s rendered with routing.provider=none: %v", kind, got[kind])
		}
	}
	if len(got["Deployment"]) != 2 || len(got["Service"]) != 2 {
		t.Fatalf("want 2 Deployments and 2 Services for poolSize 2, got %v / %v", got["Deployment"], got["Service"])
	}
}

func TestShardChartGKERouting(t *testing.T) {
	got := render(t, shardChart, "cardinal-shard/examples/hosted-gke.yaml")
	for _, kind := range []string{"HTTPRoute", "HealthCheckPolicy", "GCPBackendPolicy"} {
		if len(got[kind]) != 3 {
			t.Fatalf("want 3 %s for poolSize 3, got %v", kind, got[kind])
		}
	}
}

// envOf returns the shard container env of one rendered Deployment as name→value.
// valueFrom entries map to "<secret>/<key>".
func envOf(t *testing.T, chartDir, valuesFile, deployment string) map[string]string {
	t.Helper()
	return envOfVals(t, chartDir, loadValues(t, valuesFile), deployment)
}

func envOfVals(t *testing.T, chartDir string, vals map[string]any, deployment string) map[string]string {
	t.Helper()
	files, err := renderFiles(chartDir, vals)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range files {
		for doc := range strings.SplitSeq(body, "\n---") {
			var dep struct {
				Kind     string `json:"kind"`
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
				Spec struct {
					Template struct {
						Spec struct {
							Containers []struct {
								Env []struct {
									Name      string `json:"name"`
									Value     string `json:"value"`
									ValueFrom *struct {
										SecretKeyRef struct{ Name, Key string } `json:"secretKeyRef"`
									} `json:"valueFrom"`
								} `json:"env"`
							} `json:"containers"`
						} `json:"spec"`
					} `json:"template"`
				} `json:"spec"`
			}
			if yaml.Unmarshal([]byte(doc), &dep) != nil || dep.Kind != "Deployment" || dep.Metadata.Name != deployment {
				continue
			}
			out := map[string]string{}
			for _, e := range dep.Spec.Template.Spec.Containers[0].Env {
				if e.ValueFrom != nil {
					out[e.Name] = e.ValueFrom.SecretKeyRef.Name + "/" + e.ValueFrom.SecretKeyRef.Key
				} else {
					out[e.Name] = e.Value
				}
			}
			return out
		}
	}
	t.Fatalf("deployment %s not rendered", deployment)
	return nil
}

func TestShardChartAuthDebugLocal(t *testing.T) {
	env := envOf(t, shardChart, "cardinal-shard/examples/local.yaml", "gameplay-dpl")
	if env["CARDINAL_AUTH_MODE"] != "DEV" {
		t.Fatalf("CARDINAL_AUTH_MODE=%q", env["CARDINAL_AUTH_MODE"])
	}
	if _, ok := env["CARDINAL_ARGUS_AUTH_URL"]; ok {
		t.Fatal("CARDINAL_ARGUS_AUTH_URL set in DEV mode")
	}
	if env["CARDINAL_DEBUG"] != "true" {
		t.Fatalf("debug=%q", env["CARDINAL_DEBUG"])
	}
}

func TestShardChartAuthHosted(t *testing.T) {
	env := envOf(t, shardChart, "cardinal-shard/examples/hosted-gke.yaml", "gameplay-dpl")
	if env["CARDINAL_AUTH_MODE"] != "ARGUS" ||
		env["CARDINAL_ARGUS_AUTH_URL"] != "http://auth-backend.world-core.svc.cluster.local:8080" {
		t.Fatalf("auth env: %v", env)
	}
	if env["CARDINAL_DEBUG"] != "false" {
		t.Fatalf("CARDINAL_DEBUG=%q, hosted must be off", env["CARDINAL_DEBUG"])
	}
}

func TestShardChartAuthModeRequired(t *testing.T) {
	vals := minimal()
	delete(vals, "auth")
	renderErr(t, shardChart, vals, "auth")
}

func TestShardChartArgusNeedsURL(t *testing.T) {
	vals := minimal()
	vals["auth"] = map[string]any{"mode": "ARGUS"}
	renderErr(t, shardChart, vals, "argusURL")
}

func TestShardChartDatabaseDSN(t *testing.T) {
	env := envOf(t, shardChart, "cardinal-shard/examples/local.yaml", "gameplay-dpl")
	if !strings.HasPrefix(env["DB_DSN"], "postgres://postgres:postgres@") {
		t.Fatalf("DB_DSN=%q", env["DB_DSN"])
	}
}

func TestShardChartDatabaseSecret(t *testing.T) {
	env := envOf(t, shardChart, "cardinal-shard/examples/hosted-gke.yaml", "gameplay-dpl")
	if env["DB_DSN"] != "rampage-shard-secret/SHARD_DB_DSN" {
		t.Fatalf("DB_DSN=%q", env["DB_DSN"])
	}
}

func TestShardChartDatabaseAbsent(t *testing.T) {
	env := envOfVals(t, shardChart, minimal(), "a-dpl")
	if _, ok := env["DB_DSN"]; ok {
		t.Fatal("DB_DSN set without database values")
	}
}

func TestShardChartDatabaseBothRejected(t *testing.T) {
	vals := minimal()
	vals["database"] = map[string]any{"dsn": "postgres://x", "secret": map[string]any{"name": "s", "key": "k"}}
	renderErr(t, shardChart, vals, "database")
}

func TestShardChartNatsAndOtlp(t *testing.T) {
	local := envOf(t, shardChart, "cardinal-shard/examples/local.yaml", "gameplay-dpl")
	if local["NATS_URL"] != "nats://nats.nats.svc.cluster.local:4222" {
		t.Fatalf("NATS_URL=%q", local["NATS_URL"])
	}
	// Empty string, not absent: the engine treats "" as tracing off and unset as the groundcover default.
	if v, ok := local["OTEL_EXPORTER_OTLP_ENDPOINT"]; !ok || v != "" {
		t.Fatalf("OTEL_EXPORTER_OTLP_ENDPOINT=%q present=%v, want empty and present", v, ok)
	}
	if _, ok := local["NATS_CREDS"]; ok {
		t.Fatal("NATS_CREDS must not be set; the engine never reads it")
	}
	hosted := envOf(t, shardChart, "cardinal-shard/examples/hosted-gke.yaml", "gameplay-dpl")
	if hosted["OTEL_EXPORTER_OTLP_ENDPOINT"] != "groundcover-sensor.groundcover.svc.cluster.local:4317" {
		t.Fatalf("hosted OTLP=%q", hosted["OTEL_EXPORTER_OTLP_ENDPOINT"])
	}
	if hosted["OTEL_RESOURCE_ATTRIBUTES"] != "shard.id=gameplay,service.instance.id=gameplay" {
		t.Fatalf("OTEL_RESOURCE_ATTRIBUTES=%q", hosted["OTEL_RESOURCE_ATTRIBUTES"])
	}
}

func TestShardChartIdentityRequired(t *testing.T) {
	vals := minimal()
	delete(vals, "organization")
	renderErr(t, shardChart, vals, "organization")
}

func TestShardChartUnknownKeyRejected(t *testing.T) {
	vals := minimal()
	vals["natsURL"] = "typo"
	renderErr(t, shardChart, vals, "natsURL")
}

// Helm adds a "global" key to every values set before validation; the schema must allow it
// or additionalProperties:false rejects every install.
func TestShardChartMinimalPasses(t *testing.T) {
	if _, err := renderFiles(shardChart, minimal()); err != nil {
		t.Fatal(err)
	}
}

func TestShardChartConfigMapOptional(t *testing.T) {
	got := render(t, shardChart, "cardinal-shard/examples/local.yaml")
	if len(got["ConfigMap"]) != 1 || got["ConfigMap"][0] != "cardinal-pool-gameplay" {
		t.Fatalf("ConfigMap: %v", got["ConfigMap"])
	}
}

func TestShardChartInstanceNames(t *testing.T) {
	got := render(t, shardChart, "cardinal-shard/examples/hosted-gke.yaml")
	want := []string{"gameplay-dpl", "gameplay-2-dpl", "gameplay-3-dpl"}
	if !sameSet(got["Deployment"], want) {
		t.Fatalf("Deployments=%v want %v", got["Deployment"], want)
	}
	if !sameSet(got["Service"], []string{"gameplay", "gameplay-2", "gameplay-3"}) {
		t.Fatalf("Services=%v", got["Service"])
	}
	if !sameSet(got["PodDisruptionBudget"], []string{"gameplay", "gameplay-2", "gameplay-3"}) {
		t.Fatalf("PDBs=%v", got["PodDisruptionBudget"])
	}
}

func TestShardChartPoolSizeZero(t *testing.T) {
	vals := minimal()
	vals["poolSize"] = 0
	got := renderVals(t, shardChart, vals)
	if len(got["Deployment"]) != 0 || len(got["ConfigMap"]) != 1 {
		t.Fatalf("poolSize 0: %v", got)
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, s := range a {
		seen[s] = true
	}
	for _, s := range b {
		if !seen[s] {
			return false
		}
	}
	return true
}

func TestNatsChartRendersSingleNode(t *testing.T) {
	got := render(t, "nats", "nats/values.yaml")
	if len(got["StatefulSet"]) != 1 {
		t.Fatalf("StatefulSet=%v", got["StatefulSet"])
	}
	if !sameSet(got["Service"], []string{"nats", "nats-headless"}) {
		t.Fatalf("Services=%v", got["Service"])
	}
}

func TestPostgresChart(t *testing.T) {
	got := renderVals(t, "postgres", map[string]any{"database": "rampage"})
	for _, kind := range []string{"Deployment", "Service", "PersistentVolumeClaim"} {
		if len(got[kind]) != 1 || got[kind][0] != "test" {
			t.Fatalf("%s=%v, want one named after the release", kind, got[kind])
		}
	}
}

func TestPostgresChartDatabaseRequired(t *testing.T) {
	renderErr(t, "postgres", map[string]any{}, "database")
}

func TestPostgresChartServiceType(t *testing.T) {
	files, err := renderFiles("postgres",
		map[string]any{"database": "x", "service": map[string]any{"type": "LoadBalancer"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files["postgres/templates/postgres.yaml"], "type: LoadBalancer") {
		t.Fatal("service.type not applied")
	}
}

func TestShardChartRoutingPathIsSanitized(t *testing.T) {
	vals := minimal()
	vals["organization"] = "Argus Labs"
	vals["project"] = "My_Game"
	vals["routing"] = map[string]any{
		"provider": "gke",
		"gateway":  map[string]any{"name": "cardinal", "hostnames": []any{"x.example"}},
	}
	files, err := renderFiles(shardChart, vals)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files["cardinal-shard/templates/routing-gke.yaml"], "value: /argus-labs/my-game/a/") {
		t.Fatalf("path not sanitized:\n%s", files["cardinal-shard/templates/routing-gke.yaml"])
	}
}

func TestShardChartLocalExampleHasNoRouting(t *testing.T) {
	got := render(t, shardChart, "cardinal-shard/examples/local.yaml")
	for _, kind := range []string{"HTTPRoute", "HealthCheckPolicy", "GCPBackendPolicy", "IngressRoute", "Middleware"} {
		if len(got[kind]) != 0 {
			t.Fatalf("%s rendered for the local example", kind)
		}
	}
	files, err := renderFiles(shardChart, loadValues(t, "cardinal-shard/examples/local.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files["cardinal-shard/templates/shards.yaml"], `value: "JETSTREAM"`) {
		t.Fatal("local example must snapshot to JetStream")
	}
}

func TestShardChartGKEGatewayNamespace(t *testing.T) {
	vals := loadValues(t, "cardinal-shard/examples/hosted-gke.yaml")
	files, err := renderFiles(shardChart, vals)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(files["cardinal-shard/templates/routing-gke.yaml"], "namespace:") {
		t.Fatal("parentRef namespace rendered without routing.gateway.namespace")
	}
	vals["routing"].(map[string]any)["gateway"].(map[string]any)["namespace"] = "rampage"
	files, err = renderFiles(shardChart, vals)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files["cardinal-shard/templates/routing-gke.yaml"], "      namespace: rampage\n") {
		t.Fatal("parentRef namespace missing")
	}
}

// TestNatsChartPinsVersion keeps the vendored chart's image on the version pkg/version pins.
func TestNatsChartPinsVersion(t *testing.T) {
	data, err := os.ReadFile("nats/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "tag: "+version.Nats) {
		t.Fatalf("nats/values.yaml must pin tag: %s", version.Nats)
	}
}
