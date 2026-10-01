package cardinaloperator

import (
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/cli/pkg/version"
)

// TestEmbeddedManifests guards the embedded bundle: cluster bootstrap applies
// these manifests before anything else, so a missing or empty file (e.g. a
// broken CI sync) must fail the build, not surface at runtime.
func TestEmbeddedManifests(t *testing.T) {
	required := []string{
		"manifests/crd/cardinal.argus.gg_shardpools.yaml",
		"manifests/operator/operator.yaml",
		"manifests/nats/nats.yaml",
		"manifests/traefik/traefik.yaml",
	}

	for _, path := range required {
		data, err := Manifests.ReadFile(path)
		if err != nil {
			t.Fatalf("manifest not embedded at %s: %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("embedded manifest %s is empty", path)
		}
	}
}

// TestEmbeddedImageTagsMatchVersionPin guards against drift between
// pkg/version/version.go (single source of truth for platform versions)
// and the image tags baked into the embedded manifests. Bumping a version
// constant must update the corresponding manifest in lockstep; this test
// fails loudly if you forget.
func TestEmbeddedImageTagsMatchVersionPin(t *testing.T) {
	cases := []struct {
		manifest string
		want     string
	}{
		{"manifests/nats/nats.yaml", "image: nats:" + version.Nats},
		{"manifests/operator/operator.yaml", "/apps/cardinal-operator:" + version.CardinalOperator},
	}
	for _, c := range cases {
		data, err := Manifests.ReadFile(c.manifest)
		if err != nil {
			t.Fatalf("reading %s: %v", c.manifest, err)
		}
		if !strings.Contains(string(data), c.want) {
			t.Fatalf("%s: expected to contain %q (drift from pkg/version)", c.manifest, c.want)
		}
	}
}
