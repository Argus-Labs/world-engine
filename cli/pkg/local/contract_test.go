package local_test

import (
	"fmt"
	"maps"
	"os"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"sigs.k8s.io/yaml"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

const (
	chartDir   = "../k8s/charts/cardinal-shard"
	localVals  = chartDir + "/examples/local.yaml"
	natsHost   = "nats://nats.nats.svc.cluster.local:4222"
	dbHostK8s  = "rampage-db.rampage.svc.cluster.local"
	dbHostDock = "rampage-db"
)

// localOnlyEnv is the one key world start sets that the chart does not render.
var localOnlyEnv = map[string]bool{"LOG_FORMAT": true}

type deployment struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []struct {
					Env []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"env"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
}

// renderChartEnv returns the env of every Deployment the chart renders for examples/local.yaml, by instance.
func renderChartEnv(t *testing.T) (map[string]any, map[string]map[string]string) {
	t.Helper()
	raw, err := os.ReadFile(localVals)
	if err != nil {
		t.Fatal(err)
	}
	var vals map[string]any
	if err := yaml.Unmarshal(raw, &vals); err != nil {
		t.Fatal(err)
	}
	chrt, err := loader.Load(chartDir)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := chartutil.ToRenderValues(chrt, vals,
		chartutil.ReleaseOptions{Name: "gameplay", Namespace: "rampage"}, chartutil.DefaultCapabilities)
	if err != nil {
		t.Fatal(err)
	}
	files, err := engine.Render(chrt, merged)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]string{}
	for doc := range strings.SplitSeq(files["cardinal-shard/templates/shards.yaml"], "\n---") {
		var d deployment
		if err := yaml.Unmarshal([]byte(doc), &d); err != nil || d.Kind != "Deployment" {
			continue
		}
		env := map[string]string{}
		for _, e := range d.Spec.Template.Spec.Containers[0].Env {
			env[e.Name] = e.Value
		}
		out[strings.TrimSuffix(d.Metadata.Name, "-dpl")] = env
	}
	return vals, out
}

func dockerEnv(t *testing.T, vals map[string]any) map[string]map[string]string {
	t.Helper()
	size, _ := vals["poolSize"].(float64)
	tick, _ := vals["tickRate"].(float64)
	debug, _ := vals["debug"].(bool)
	shardID, _ := vals["shardID"].(string)
	org, _ := vals["organization"].(string)
	project, _ := vals["project"].(string)
	cfg := &service.Config{
		Project: project,
		NATSURL: service.NatsURL(project),
		Debug:   debug,
		WorldToml: worldtoml.Config{
			Organization: org,
			Project:      project,
			Services:     []worldtoml.GameService{{ID: "meta", Path: "services/meta/cmd", DB: true}},
		},
	}
	out := map[string]map[string]string{}
	for i := range int(size) {
		instance := shardID
		if i > 0 {
			instance = fmt.Sprintf("%s-%d", shardID, i+1) // matches the chart's instanceName helper
		}
		svc := service.CardinalFromShard(
			cfg,
			worldtoml.Shard{ID: shardID, InstanceID: instance, TickRate: int32(tick)},
			service.ShardHostPort(i),
		)
		env := map[string]string{}
		for _, kv := range svc.Env {
			k, v, _ := strings.Cut(kv, "=")
			env[k] = v
		}
		out[instance] = env
	}
	return out
}

// normalize maps the two hosts that legitimately differ between a cluster and Docker onto one spelling.
func normalize(env map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range env {
		if localOnlyEnv[k] {
			continue
		}
		v = strings.ReplaceAll(v, natsHost, "NATS")
		v = strings.ReplaceAll(v, "nats://rampage-nats:4222", "NATS")
		v = strings.ReplaceAll(v, dbHostK8s, "DB")
		v = strings.ReplaceAll(v, "@"+dbHostDock+":", "@DB:")
		out[k] = v
	}
	return out
}

// TestDockerEnvMatchesChartLocalExample is the ADR-066 v7 contract: world start
// gives a shard the same environment the chart renders for examples/local.yaml.
func TestDockerEnvMatchesChartLocalExample(t *testing.T) {
	vals, chart := renderChartEnv(t)
	if len(chart) < 2 {
		t.Fatalf("expected a pool of >= 2 in the local example, got %v", chart)
	}
	docker := dockerEnv(t, vals)
	for instance, chartEnv := range chart {
		dockerEnv, ok := docker[instance]
		if !ok {
			t.Fatalf("docker side has no instance %q (have %v)", instance, maps.Keys(docker))
		}
		got, want := normalize(dockerEnv), normalize(chartEnv)
		if !maps.Equal(got, want) {
			t.Errorf("%s: env differs\n docker: %v\n chart:  %v", instance, got, want)
		}
	}
}

// TestLocalExampleHasNoRouting pins that local relies on the edge, not a cluster route.
func TestLocalExampleHasNoRouting(t *testing.T) {
	vals, _ := renderChartEnv(t)
	routing, _ := vals["routing"].(map[string]any)
	if routing["provider"] != "none" {
		t.Fatalf("examples/local.yaml routing.provider = %v, want none", routing["provider"])
	}
}

// TestArgusAuthEnvMatchesChart holds the other half of the auth contract: a world
// running ARGUS locally gets the same two variables the chart renders for a cluster.
func TestArgusAuthEnvMatchesChart(t *testing.T) {
	const argusURL = "https://api.argus.dev"

	chrt, err := loader.Load(chartDir)
	if err != nil {
		t.Fatal(err)
	}
	vals := map[string]any{
		"shardID": "gameplay", "image": "x", "imageTag": "y", "poolSize": 1,
		"organization": "argus", "project": "rampage", "region": "us-west1",
		"nats": map[string]any{"url": "nats://n:4222"},
		"auth": map[string]any{"mode": "ARGUS", "argusURL": argusURL},
	}
	merged, err := chartutil.ToRenderValues(chrt, vals,
		chartutil.ReleaseOptions{Name: "gameplay", Namespace: "rampage"}, chartutil.DefaultCapabilities)
	if err != nil {
		t.Fatal(err)
	}
	files, err := engine.Render(chrt, merged)
	if err != nil {
		t.Fatal(err)
	}

	chartEnv := map[string]string{}
	for doc := range strings.SplitSeq(files["cardinal-shard/templates/shards.yaml"], "\n---") {
		var d deployment
		if err := yaml.Unmarshal([]byte(doc), &d); err != nil || d.Kind != "Deployment" {
			continue
		}
		for _, e := range d.Spec.Template.Spec.Containers[0].Env {
			if strings.HasPrefix(e.Name, "CARDINAL_AUTH") || strings.HasPrefix(e.Name, "CARDINAL_ARGUS") {
				chartEnv[e.Name] = e.Value
			}
		}
	}

	cfg := &service.Config{
		Project: "rampage",
		NATSURL: service.NatsURL("rampage"),
		WorldToml: worldtoml.Config{
			Organization: "argus",
			Project:      "rampage",
			Auth:         worldtoml.Auth{Mode: worldtoml.AuthModeArgus, URL: argusURL},
		},
	}
	svc := service.CardinalFromShard(
		cfg,
		worldtoml.Shard{ID: "gameplay", InstanceID: "gameplay"},
		service.ShardHostPort(0),
	)
	dockerEnv := map[string]string{}
	for _, kv := range svc.Env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "CARDINAL_AUTH") || strings.HasPrefix(k, "CARDINAL_ARGUS") {
			dockerEnv[k] = v
		}
	}

	want := map[string]string{"CARDINAL_AUTH_MODE": "ARGUS", "CARDINAL_ARGUS_AUTH_URL": argusURL}
	if !maps.Equal(chartEnv, want) || !maps.Equal(dockerEnv, want) {
		t.Fatalf("auth env differs\n chart:  %v\n docker: %v\n want:   %v", chartEnv, dockerEnv, want)
	}
}
