//nolint:testpackage // exercises unexported shardPoolYAML helper directly
package cluster

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// parseShardPoolCR round-trips the rendered document back through a YAML
// decoder. Asserting on the decoded value (rather than raw substrings) keeps
// the tests agnostic to the encoder's quoting choices and proves the output is
// valid, parseable YAML.
func parseShardPoolCR(t *testing.T, doc []byte) shardPoolCR {
	t.Helper()
	var cr shardPoolCR
	require.NoError(t, yaml.Unmarshal(doc, &cr))
	return cr
}

func TestShardPoolYAML_MinimalShardEmitsResourcesAndCorrectMetadata(t *testing.T) {
	t.Parallel()

	p := ShardPool{
		ShardID:      "gameplay",
		Organization: "argus",
		Project:      "rampage",
		Region:       DefaultRegion,
		Image:        "k3d-world-engine-registry.localhost:5000/rampage/gameplay",
		PoolSize:     1,
		Resources:    DefaultResources(),
	}

	doc, err := shardPoolYAML(p)
	require.NoError(t, err)

	cr := parseShardPoolCR(t, doc)
	require.Equal(t, "cardinal.argus.gg/v1", cr.APIVersion)
	require.Equal(t, "ShardPool", cr.Kind)
	require.Equal(t, "gameplay", cr.Metadata.Name)
	require.Equal(t, operatorNamespace, cr.Metadata.Namespace)
	require.Equal(t, "gameplay", cr.Spec.ShardID)
	require.Equal(t, "k3d-world-engine-registry.localhost:5000/rampage/gameplay", cr.Spec.Image)
	require.EqualValues(t, 1, cr.Spec.PoolSize)

	// resources block always present (CRD requires it).
	require.EqualValues(t, 250, cr.Spec.Resources.Requests.CPU)
	require.EqualValues(t, 256, cr.Spec.Resources.Requests.Memory)
	require.EqualValues(t, 1000, cr.Spec.Resources.Limits.CPU)
	require.EqualValues(t, 1024, cr.Spec.Resources.Limits.Memory)
}

func TestShardPoolYAML_OmitsUnsetOptionalFields(t *testing.T) {
	t.Parallel()

	p := ShardPool{
		ShardID:      "meta",
		Organization: "argus",
		Project:      "rampage",
		Region:       DefaultRegion,
		Image:        "k3d-r.localhost:5000/rampage/meta",
		PoolSize:     1,
		Resources:    DefaultResources(),
		// TickRate, Mode, LogLevel intentionally zero/empty.
	}

	doc, err := shardPoolYAML(p)
	require.NoError(t, err)
	out := string(doc)

	require.NotContains(t, out, "tickRate:")
	require.NotContains(t, out, "mode:")
	require.NotContains(t, out, "logLevel:")
}

func TestShardPoolYAML_IncludesOptionalFieldsWhenSet(t *testing.T) {
	t.Parallel()

	p := ShardPool{
		ShardID:      "gameplay",
		Organization: "argus",
		Project:      "rampage",
		Region:       DefaultRegion,
		Image:        "k3d-r.localhost:5000/rampage/gameplay",
		TickRate:     20,
		Mode:         "LEADER",
		LogLevel:     "warn",
		PoolSize:     3,
		Resources:    DefaultResources(),
	}

	doc, err := shardPoolYAML(p)
	require.NoError(t, err)

	cr := parseShardPoolCR(t, doc)
	require.EqualValues(t, 20, cr.Spec.TickRate)
	require.Equal(t, "LEADER", cr.Spec.Mode)
	require.Equal(t, "warn", cr.Spec.LogLevel)
	require.EqualValues(t, 3, cr.Spec.PoolSize)
}

// TestShardPoolYAML_EscapesStringsWithQuotes is the regression test for the bug
// this encoder swap fixes: a config-sourced string containing a double quote
// (e.g. `mode = "LEADER\"hello"` in world.toml) must round-trip intact rather
// than producing malformed YAML like `mode: "LEADER"hello"`.
func TestShardPoolYAML_EscapesStringsWithQuotes(t *testing.T) {
	t.Parallel()

	const nasty = `LEADER"hello`
	p := ShardPool{
		ShardID:      "gameplay",
		Organization: "argus",
		Project:      "rampage",
		Region:       DefaultRegion,
		Image:        "k3d-r.localhost:5000/rampage/gameplay",
		Mode:         nasty,
		PoolSize:     1,
		Resources:    DefaultResources(),
	}

	doc, err := shardPoolYAML(p)
	require.NoError(t, err)

	// The whole document must still parse, and Mode must come back byte-for-byte.
	cr := parseShardPoolCR(t, doc)
	require.Equal(t, nasty, cr.Spec.Mode)

	// And the unescaped, ambiguous form must never appear in the output.
	require.NotContains(t, string(doc), `mode: "LEADER"hello"`)
}

// TestShardPoolYAML_EscapesStructuralInjection covers the strongest injection
// vectors a line-oriented hand-template was vulnerable to — a newline (which
// could forge a sibling YAML key) or a leading YAML indicator character — in
// any config-sourced string field, not just Mode. Each must round-trip
// byte-for-byte through the encoder, and must not alter any sibling field.
func TestShardPoolYAML_EscapesStructuralInjection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		val  string
		set  func(p *ShardPool, v string)
		get  func(cr shardPoolCR) string
	}{
		{
			"image_newline_forges_key",
			"evil\npoolSize: 999",
			func(p *ShardPool, v string) { p.Image = v },
			func(cr shardPoolCR) string { return cr.Spec.Image },
		},
		{
			"shardID_newline_forges_key",
			"evil\nmode: INJECTED",
			func(p *ShardPool, v string) { p.ShardID = v },
			func(cr shardPoolCR) string { return cr.Spec.ShardID },
		},
		{
			"org_leading_indicator",
			"!!evil",
			func(p *ShardPool, v string) { p.Organization = v },
			func(cr shardPoolCR) string { return cr.Spec.Organization },
		},
		{
			"region_colon_space",
			"a: b",
			func(p *ShardPool, v string) { p.Region = v },
			func(cr shardPoolCR) string { return cr.Spec.Region },
		},
		{
			"logLevel_leading_hash",
			"#info",
			func(p *ShardPool, v string) { p.LogLevel = v },
			func(cr shardPoolCR) string { return cr.Spec.LogLevel },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := ShardPool{
				ShardID:      "gameplay",
				Organization: "argus",
				Project:      "rampage",
				Region:       DefaultRegion,
				Image:        "k3d-r.localhost:5000/rampage/gameplay",
				PoolSize:     1,
				Resources:    DefaultResources(),
			}
			tc.set(&p, tc.val)

			doc, err := shardPoolYAML(p)
			require.NoError(t, err)

			cr := parseShardPoolCR(t, doc)
			// The hostile value must round-trip byte-for-byte...
			require.Equal(t, tc.val, tc.get(cr))
			// ...and a forged structural payload must not have altered siblings.
			require.EqualValues(t, 1, cr.Spec.PoolSize)
		})
	}
}

// TestShardPoolYAML_ParsesAsSingleDocument guards the contract applyYAML relies
// on: a single, decodable YAML document.
func TestShardPoolYAML_ParsesAsSingleDocument(t *testing.T) {
	t.Parallel()

	doc, err := shardPoolYAML(ShardPool{
		ShardID:   "gameplay",
		PoolSize:  1,
		Resources: DefaultResources(),
	})
	require.NoError(t, err)

	docs := 0
	for d := range strings.SplitSeq(string(doc), "\n---") {
		if strings.TrimSpace(d) != "" {
			docs++
		}
	}
	require.Equal(t, 1, docs)
}

// patchOperatorEnv injects into the container env block; the value is YAML-quoted.
func TestPatchOperatorEnv(t *testing.T) {
	t.Parallel()

	doc := []byte("spec:\n  containers:\n  - name: manager\n    env:\n" +
		"    - name: NATS_URL\n      value: nats://x:4222\n")
	dsn := "postgres://u:p@h:5432/d?sslmode=disable"
	got := string(patchOperatorEnv(doc, "SHARD_DB_DSN", dsn))

	require.Contains(t, got, "    - name: SHARD_DB_DSN\n      value: \""+dsn+"\"\n")
	require.Contains(t, got, "- name: NATS_URL") // existing entries preserved

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal(patchOperatorEnv(doc, "A", "b"), &parsed)) // stays valid YAML
}
