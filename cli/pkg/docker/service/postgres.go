package service

import (
	"fmt"
	"strconv"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

// ProjectDBVolumeName is the named volume holding "<project>-db" data; purge removes it.
func ProjectDBVolumeName(project string) string { return ProjectDBContainerName(project) + "-data" }

// ProjectDBService is the per-project Postgres every shard reaches through DB_DSN,
// published on 127.0.0.1:5432. Added when NeedsAutoProjectDB(worldToml) is true.
func ProjectDBService(cfg *Config) Service {
	project := cfg.WorldToml.Project
	name := ProjectDBContainerName(project)
	tcp := network.MustParsePort(strconv.Itoa(defaultConfigDBPort) + "/tcp")
	return Service{
		Name:  name,
		Image: projectDBImage,
		Env: []string{
			"POSTGRES_USER=" + configDBUser,
			"POSTGRES_PASSWORD=" + configDBPassword,
			"POSTGRES_DB=" + project,
		},
		Labels:       Labels(project, RoleDB, "", ""),
		ExposedPorts: getExposedPorts([]int{defaultConfigDBPort}),
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD-SHELL", fmt.Sprintf("pg_isready -U %s -d %s", configDBUser, project)},
			Interval: gameServiceHealthInterval,
			Timeout:  gameServiceHealthTimeout,
			Retries:  gameServiceHealthRetries,
		},
		PortBindings: network.PortMap{
			tcp: loopbackBinding(defaultConfigDBPort),
		},
		NetworkMode: container.NetworkMode(NetworkName(project)),
		Binds:       []string{ProjectDBVolumeName(project) + ":/var/lib/postgresql/data"},
	}
}
