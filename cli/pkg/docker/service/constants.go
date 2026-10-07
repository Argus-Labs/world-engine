package service

// Common string values used across service configurations.
const (
	// ProjectLabel marks every container of one project; NetworkName(project) is its network.
	ProjectLabel = "world.argus.gg/project"
	// ShardIDLabel is the pool id of a shard container ("gameplay").
	ShardIDLabel = "world.argus.gg/shard-id"
	// InstanceLabel is the pool-expanded instance id ("gameplay-2"); also set on service containers.
	InstanceLabel = "world.argus.gg/instance"
	// RoleLabel is one of the Role* values; StartContainers orders containers by it.
	RoleLabel = "world.argus.gg/role"
	// OrgLabel is the world's organization, set on shard containers so readers without world.toml can address them.
	OrgLabel = "world.argus.gg/organization"

	RoleShard   = "shard"
	RoleNATS    = "nats"
	RoleDB      = "db"
	RoleService = "service"

	// CardinalImageLabel marks images built by `world`; purge --image prunes by it.
	CardinalImageLabel = "world-cli"
	CardinalImageValue = "cardinal"

	// BaseImage is the distroless runtime base for built-from-source images.
	BaseImage = "gcr.io/distroless/base-debian12"

	// GoBuilderImage is the Go toolchain build stage shared by Cardinal shards and
	// built-from-source game services. Keep the Go version in sync with .prototools
	// (go = "...").
	GoBuilderImage = "golang:1.27.1-bookworm"

	// CardinalRegion is injected into shard + game-service env. Hard coded for
	// local dev until multi-region is wired up.
	CardinalRegion = "us-west1"
)

// NetworkName is the per-project Docker bridge network.
func NetworkName(project string) string { return project }

// Labels returns the common label set for a container of a project.
func Labels(project, role, shardID, instance string) map[string]string {
	l := map[string]string{ProjectLabel: project, RoleLabel: role}
	if shardID != "" {
		l[ShardIDLabel] = shardID
	}
	if instance != "" {
		l[InstanceLabel] = instance
	}
	return l
}
