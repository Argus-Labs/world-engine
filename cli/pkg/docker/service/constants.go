package service

// Common string values used across service configurations.
const (
	// DefaultNetworkMode is the docker network shared by all services.
	DefaultNetworkMode = "world-engine-network"

	// CardinalNamespaceLabel labels Cardinal shard containers/images by namespace.
	CardinalNamespaceLabel = "com.world.cardinal.namespace"

	// GameServiceLabel labels [[services]] containers by project.
	GameServiceLabel = "com.world.service.project"

	// BaseImage is the distroless runtime base for built-from-source images.
	BaseImage = "gcr.io/distroless/base-debian12"

	// GoBuilderImage is the Go toolchain build stage shared by Cardinal shards and
	// built-from-source game services. Keep the Go version in sync with .prototools
	// (go = "...").
	GoBuilderImage = "golang:1.27.1-bookworm"

	// cardinalRegion is injected into shard + game-service env. Hard coded for
	// local dev until multi-region is wired up.
	cardinalRegion = "us-west1"
)
