package cluster

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// mustLoad parses an inline world.toml fragment for in-package tests of
// unexported helpers (shardpool_test.go's loadCfg lives in the external package).
// A minimal shard is appended when the fragment declares none, so service-only
// fragments satisfy toml.validate's "at least one shard" rule.
func mustLoad(t *testing.T, body string) toml.Config {
	t.Helper()
	if !strings.Contains(body, "[[shards]]") {
		body += "\n[[shards]]\nid = \"gameplay\"\n"
	}
	cfg, err := toml.Load(strings.NewReader(body))
	require.NoError(t, err)
	return cfg
}

func TestProjectDBDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "no db",
			body: `
				organization = "argus"
				project = "rampage"
				[[shards]]
				id = "gameplay"
			`,
			want: "",
		},
		{
			name: "auto db (db=true service, no config_db)",
			body: `
				organization = "argus"
				project = "rampage"
				[[services]]
				id = "gameplay"
				path = "."
				db = true
			`,
			want: "postgres://postgres:postgres@rampage-db.cardinal-operator-system.svc.cluster.local:5432/rampage?sslmode=disable",
		},
		{
			name: "config_db service preferred over auto db",
			body: `
				organization = "argus"
				project = "rampage"
				[[services]]
				id = "postgres"
				image = "postgres:16"
				config_db = true
				[[services]]
				id = "gameplay"
				path = "."
				db = true
			`,
			want: "postgres://postgres:postgres@rampage-postgres-service.cardinal-operator-system.svc.cluster.local:5432/rampage?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := mustLoad(t, tt.body)
			require.Equal(t, tt.want, projectDBDSN(cfg))
		})
	}
}

// shardDBDSN never returns "": with no [[services]] it still points at the auto project DB.
func TestShardDBDSN(t *testing.T) {
	t.Parallel()

	noServices := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[shards]]
		id = "gameplay"
	`)
	wantAuto := "postgres://postgres:postgres@rampage-db.cardinal-operator-system" +
		".svc.cluster.local:5432/rampage?sslmode=disable"
	require.Equal(t, wantAuto, shardDBDSN(noServices))

	withConfigDB := mustLoad(t, `
		organization = "argus"
		project = "rampage"
		[[services]]
		id = "postgres-service"
		image = "postgres:16"
		config_db = true
	`)
	require.Equal(t, projectDBDSN(withConfigDB), shardDBDSN(withConfigDB))
}

func TestProjectDBPredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		anyUsesDB   bool
		hasConfigDB bool
	}{
		{
			name: "no services",
			body: `
				organization = "argus"
				project = "rampage"
				[[shards]]
				id = "gameplay"
			`,
		},
		{
			name: "db=true service only",
			body: `
				organization = "argus"
				project = "rampage"
				[[services]]
				id = "gameplay"
				path = "."
				db = true
			`,
			anyUsesDB: true,
		},
		{
			name: "config_db satisfies db",
			body: `
				organization = "argus"
				project = "rampage"
				[[services]]
				id = "postgres"
				image = "postgres:16"
				config_db = true
				[[services]]
				id = "gameplay"
				path = "."
				db = true
			`,
			anyUsesDB:   true,
			hasConfigDB: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := mustLoad(t, tt.body)
			require.Equal(t, tt.anyUsesDB, anyServiceUsesDB(cfg))
			require.Equal(t, tt.hasConfigDB, hasConfigDBService(cfg))
		})
	}
}
