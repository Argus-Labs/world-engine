package service_test

import (
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

func TestVolumeNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		services []worldtoml.GameService
		want     []string
	}{
		{
			name: "shards only",
			want: []string{"demo-nats-data", "demo-db-data"},
		},
		{
			name: "image and path services with volumes, in declaration order",
			services: []worldtoml.GameService{
				{ID: "meta", Image: "postgres:16", Volume: "/var/lib/postgresql/data"},
				{ID: "chat", Path: "services/chat", Volume: "/data"},
			},
			want: []string{"demo-nats-data", "demo-db-data", "demo-meta-service", "demo-chat-service"},
		},
		{
			name: "service without a volume contributes nothing",
			services: []worldtoml.GameService{
				{ID: "stateless", Image: "redis:7"},
				{ID: "meta", Image: "postgres:16", Volume: "/data"},
			},
			want: []string{"demo-nats-data", "demo-db-data", "demo-meta-service"},
		},
		{
			// The auto DB volume stays listed: it may predate the config_db service.
			name: "config_db service keeps the auto db volume",
			services: []worldtoml.GameService{
				{
					ID:       "postgres",
					Image:    "postgres:16",
					ConfigDB: true,
					Ports:    []int{5432},
					Volume:   "/var/lib/postgresql/data",
				},
			},
			want: []string{"demo-nats-data", "demo-db-data", "demo-postgres-service"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &service.Config{WorldToml: worldtoml.Config{
				Project:  "demo",
				Shards:   []worldtoml.Shard{{ID: "game", InstanceID: "game", Path: "shards/game"}},
				Services: tc.services,
			}}
			got := service.VolumeNames(cfg)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("VolumeNames = %v, want %v", got, tc.want)
			}
		})
	}
}
